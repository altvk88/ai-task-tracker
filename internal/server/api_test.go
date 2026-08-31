package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alkulagin-creator/tt/internal/index"
)

// newTestVault раскладывает временный vault с двумя нормальными тасками и
// одной битой (без фронтматтера) — этого достаточно, чтобы проверить и
// happy path, и попадание сломанной таски в снимок.
func newTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tasksDir := filepath.Join(dir, "tasks", "demo")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(tasksDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	write("demo-001.md", "---\n"+
		"id: DEMO-001\n"+
		"title: Первая таска\n"+
		"status: ready\n"+
		"project: demo\n"+
		"priority: high\n"+
		"created: 2026-08-01\n"+
		"---\n\n## Description\n\nТело первой таски.\n")

	write("demo-002.md", "---\n"+
		"id: DEMO-002\n"+
		"title: Вторая таска\n"+
		"status: in-progress\n"+
		"project: demo\n"+
		"priority: low\n"+
		"created: 2026-08-02\n"+
		"---\n\n## Description\n\nВторая.\n")

	write("demo-broken.md", "# Нет фронтматтера вообще\n")

	return dir
}

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	vaultDir := newTestVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	return New(ix, vaultDir, Options{}), vaultDir
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, ожидался JSON", ct)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("тело не разбирается как JSON (%v): %s", err, rr.Body.String())
	}
}

func TestSnapshot_всеТаскиВключаяБитую(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	var resp snapshotResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Tasks) != 3 {
		t.Fatalf("тасок в снимке %d, ожидалось 3", len(resp.Tasks))
	}
	if resp.Summary.Total != 3 {
		t.Errorf("Summary.Total = %d, ожидалось 3", resp.Summary.Total)
	}
	if resp.Summary.Broken != 1 {
		t.Errorf("Summary.Broken = %d, ожидалось 1", resp.Summary.Broken)
	}

	var brokenSeen bool
	for _, task := range resp.Tasks {
		if task.ParseErr != "" {
			brokenSeen = true
		}
	}
	if !brokenSeen {
		t.Errorf("битая таска не найдена в снимке или у неё пустой ParseErr")
	}
}

func TestSnapshot_схемаСохраняетПорядокЛейнов(t *testing.T) {
	srv, vaultDir := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	var resp snapshotResponse
	decodeJSON(t, rr, &resp)

	if resp.Schema == nil || len(resp.Schema.Statuses) == 0 {
		t.Fatalf("схема в ответе пуста")
	}

	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	want := ix.Schema().Statuses
	if len(resp.Schema.Statuses) != len(want) {
		t.Fatalf("статусов в ответе %d, в схеме индекса %d", len(resp.Schema.Statuses), len(want))
	}
	for i := range want {
		if resp.Schema.Statuses[i].ID != want[i].ID || resp.Schema.Statuses[i].Lane != want[i].Lane {
			t.Errorf("статус %d: получено %+v, ожидалось %+v", i, resp.Schema.Statuses[i], want[i])
		}
	}
}

func TestSnapshot_методНеПоддерживается(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	// RemoteAddr — loopback: тест проверяет 405 на неверный метод, а не 401
	// авторизации (у неё дефолтный httptest.NewRequest адрес не loopback).
	req := httptest.NewRequest(http.MethodPost, "/api/snapshot", nil)
	req.RemoteAddr = "127.0.0.1:1"
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код = %d, ожидался 405", rr.Code)
	}
	var errResp errorResponse
	decodeJSON(t, rr, &errResp)
	if errResp.Error == "" {
		t.Errorf("поле error пустое")
	}
}

func TestTask_телоБезФронтматтера(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task/DEMO-001", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	var detail taskDetail
	decodeJSON(t, rr, &detail)

	if detail.ID != "DEMO-001" {
		t.Errorf("ID = %q", detail.ID)
	}
	want := "## Description\n\nТело первой таски.\n"
	if detail.Body != want {
		t.Errorf("Body = %q, ожидалось %q", detail.Body, want)
	}
}

func TestTask_несуществующийID(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task/NOPE-999", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("код = %d, ожидался 404, тело: %s", rr.Code, rr.Body.String())
	}
	var errResp errorResponse
	decodeJSON(t, rr, &errResp)
	if errResp.Error == "" {
		t.Errorf("поле error пустое")
	}
}

func TestTask_методНеПоддерживается(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	// RemoteAddr — loopback: тест проверяет 405 на неверный метод, а не 401
	// авторизации (у неё дефолтный httptest.NewRequest адрес не loopback).
	req := httptest.NewRequest(http.MethodPost, "/api/task/DEMO-001", nil)
	req.RemoteAddr = "127.0.0.1:1"
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код = %d, ожидался 405, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestНеизвестныйМаршрут(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/nope", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("код = %d, ожидался 404", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, ожидался JSON даже для неизвестного маршрута", ct)
	}
}
