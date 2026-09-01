package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/index"
)

// newCreateVault — vault с проектом demo и штатным шаблоном таски, но без
// готовых тасок: создание проверяется на чистом каталоге.
func newCreateVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"projects/demo.md": "---\nproject: demo\nid_prefix: W\nnext_id: 1\n---\n",
		"templates/task-template.md": "---\n" +
			"id:\n" +
			"title: \"<% tp.file.title %>\"\n" +
			"status: backlog\n" +
			"project:\n" +
			"priority: medium\n" +
			"due:\n" +
			"created: <% tp.date.now(\"YYYY-MM-DD\") %>\n" +
			"completed:\n" +
			"blocked_by:\n" +
			"effort:\n" +
			"attempts: 0\n" +
			"spec:\n" +
			"result:\n" +
			"claim:\n" +
			"---\n\n## Description\n\n\n## Log\n\n- <% tp.date.now(\"YYYY-MM-DD\") %>: Created\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "tasks", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func postCreate(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/task", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestCreateTask_создаётСIDОтTaskop(t *testing.T) {
	vaultDir := newCreateVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ix, vaultDir, Options{})

	rr := postCreate(t, srv, `{"project":"demo","title":"Новая таска с панели","priority":"high"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("код = %d, ожидался 201, тело: %s", rr.Code, rr.Body.String())
	}
	var got createTaskResponse
	decodeJSON(t, rr, &got)
	if got.ID != "W-001" {
		t.Fatalf("ID = %q, ожидался W-001 (выдаёт taskop.New)", got.ID)
	}
	if got.Priority != "high" || got.Status != "ready" {
		t.Fatalf("ответ = %+v", got)
	}
	if got.Version == "" {
		t.Error("ответ обязан содержать version")
	}

	path := filepath.Join(vaultDir, "tasks", "demo", "novaya-taska-s-paneli.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл таски не создан: %v", err)
	}

	// next_id обязан увеличиться — тем же способом, каким это делает
	// taskop.New для CLI: сервер счётчик не изобретает заново.
	proj, err := os.ReadFile(filepath.Join(vaultDir, "projects", "demo.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(proj), "next_id: 2") {
		t.Fatalf("next_id не увеличен:\n%s", proj)
	}
}

func TestCreateTask_видноВСнимкеСразу(t *testing.T) {
	vaultDir := newCreateVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ix, vaultDir, Options{})

	if rr := postCreate(t, srv, `{"project":"demo","title":"Таска"}`); rr.Code != http.StatusCreated {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))
	var snap snapshotResponse
	decodeJSON(t, rr, &snap)
	found := false
	for _, task := range snap.Tasks {
		if task.ID == "W-001" {
			found = true
		}
	}
	if !found {
		t.Fatal("созданная таска не видна в /api/snapshot без ожидания fsnotify")
	}
}

func TestCreateTask_безОбязательныхПолей400(t *testing.T) {
	vaultDir := newCreateVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ix, vaultDir, Options{})

	for _, body := range []string{`{}`, `{"project":"demo"}`, `{"title":"без проекта"}`, `{`, ``} {
		if rr := postCreate(t, srv, body); rr.Code != http.StatusBadRequest {
			t.Errorf("тело %q: код = %d, ожидался 400 (%s)", body, rr.Code, rr.Body.String())
		}
	}
}

func TestCreateTask_несуществующийПроект404(t *testing.T) {
	vaultDir := newCreateVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ix, vaultDir, Options{})

	rr := postCreate(t, srv, `{"project":"выдумка","title":"Таска"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("код = %d, ожидался 404, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateTask_getДаёт405(t *testing.T) {
	vaultDir := newCreateVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ix, vaultDir, Options{})

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код = %d, ожидался 405, тело: %s", rr.Code, rr.Body.String())
	}
}
