package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/cli"
	"github.com/alkulagin-creator/tt/internal/index"
)

// writeVaultFiles — содержимое vault для тестов записи. Держим его отдельной
// таблицей, чтобы тест согласованности мог разложить два байт-в-байт
// одинаковых vault: один для CLI, другой для API.
var writeVaultFiles = map[string]string{
	"w-001.md": "---\n" +
		"id: W-001\n" +
		"title: Свободная\n" +
		"status: ready\n" +
		"project: demo\n" +
		"priority: high\n" +
		"created: 2026-08-01\n" +
		"ready_at:\n" +
		"completed:\n" +
		"claim:\n" +
		"---\n\n## Description\n\nТело.\n",
	"w-002.md": "---\n" +
		"id: W-002\n" +
		"title: Заблокированная\n" +
		"status: backlog\n" +
		"project: demo\n" +
		"priority: medium\n" +
		"created: 2026-08-01\n" +
		"blocked_by: [W-003]\n" +
		"---\n\n## Description\n\nЖдёт W-003.\n",
	"w-003.md": "---\n" +
		"id: W-003\n" +
		"title: Блокер\n" +
		"status: ready\n" +
		"project: demo\n" +
		"priority: low\n" +
		"created: 2026-08-01\n" +
		"---\n",
	// Двоеточие в незакавыченном заголовке ломает YAML: таска на месте, но
	// её фронтматтер не разбирается.
	"w-009.md": "---\n" +
		"id: W-009\n" +
		"title: Баг (прод): всё сломалось\n" +
		"status: ready\n" +
		"---\n",
}

func newWriteVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tasksDir := filepath.Join(dir, "tasks", "demo")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, content := range writeVaultFiles {
		if err := os.WriteFile(filepath.Join(tasksDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return dir
}

func newWriteServer(t *testing.T) (*Server, string) {
	t.Helper()
	vaultDir := newWriteVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	return New(ix, vaultDir, Options{}), vaultDir
}

func postStatus(t *testing.T, srv *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/task/"+id+"/status", strings.NewReader(body))
	// httptest.NewRequest ставит внешний RemoteAddr (192.0.2.1), а сервер без
	// токена удалённую запись запрещает. Эти тесты проверяют логику смены
	// статуса, а не авторизацию, поэтому запрос объявляется локальным явно.
	req.RemoteAddr = "127.0.0.1:12345"
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func taskFile(t *testing.T, vaultDir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vaultDir, "tasks", "demo", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// diffLines считает, сколько строк различается у двух версий файла.
func diffLines(t *testing.T, before, after string) int {
	t.Helper()
	b := strings.Split(before, "\n")
	a := strings.Split(after, "\n")
	if len(b) != len(a) {
		t.Fatalf("изменилось число строк: было %d, стало %d", len(b), len(a))
	}
	n := 0
	for i := range b {
		if b[i] != a[i] {
			n++
		}
	}
	return n
}

func TestPostStatus_меняетРовноОднуСтроку(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	before := taskFile(t, vaultDir, "w-001.md")

	rr := postStatus(t, srv, "W-001", `{"to":"hold"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	after := taskFile(t, vaultDir, "w-001.md")
	if n := diffLines(t, before, after); n != 1 {
		t.Fatalf("изменилось строк: %d, ожидалась одна\n%s", n, after)
	}
	if !strings.Contains(after, "status: hold") {
		t.Fatalf("статус не записан:\n%s", after)
	}

	var got apiTask
	decodeJSON(t, rr, &got)
	if got.ID != "W-001" || got.Status != "hold" {
		t.Fatalf("ответ = %+v, ожидалась обновлённая таска", got)
	}
}

// Ответ 200 обязан приходить только после того, как изменение видно в снимке:
// клиент, обновивший карточку по ответу, не должен тут же получить из
// /api/snapshot старое состояние.
func TestPostStatus_снимокУжеОбновлён(t *testing.T) {
	srv, _ := newWriteServer(t)
	if rr := postStatus(t, srv, "W-001", `{"to":"hold"}`); rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))
	var snap snapshotResponse
	decodeJSON(t, rr, &snap)
	for _, task := range snap.Tasks {
		if task.ID == "W-001" && task.Status != "hold" {
			t.Fatalf("в снимке статус %q, ожидался hold", task.Status)
		}
	}
}

func TestPostStatus_заблокированнаяТаска409ИФайлНеТронут(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	before := taskFile(t, vaultDir, "w-002.md")

	rr := postStatus(t, srv, "W-002", `{"to":"in-progress"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("код = %d, ожидался 409, тело: %s", rr.Code, rr.Body.String())
	}
	var resp errorResponse
	decodeJSON(t, rr, &resp)
	if !strings.Contains(resp.Error, "W-003") {
		t.Fatalf("ошибка обязана называть блокер, получено: %q", resp.Error)
	}
	if after := taskFile(t, vaultDir, "w-002.md"); after != before {
		t.Fatalf("отклонённый переход изменил файл:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, ".locks", "W-002.lock")); !os.IsNotExist(err) {
		t.Error("отклонённый переход не должен оставлять замок")
	}
}

func TestPostStatus_ненайденнаяТаска404(t *testing.T) {
	srv, _ := newWriteServer(t)
	if rr := postStatus(t, srv, "W-404", `{"to":"ready"}`); rr.Code != http.StatusNotFound {
		t.Fatalf("код = %d, ожидался 404, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestPostStatus_непарсящаясяТаска422(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	before := taskFile(t, vaultDir, "w-009.md")

	rr := postStatus(t, srv, "W-009", `{"to":"done"}`)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("код = %d, ожидался 422, тело: %s", rr.Code, rr.Body.String())
	}
	if after := taskFile(t, vaultDir, "w-009.md"); after != before {
		t.Fatalf("непарсящаяся таска не должна меняться:\n%s", after)
	}
}

func TestPostStatus_кривоеТело400(t *testing.T) {
	srv, _ := newWriteServer(t)
	for _, body := range []string{``, `{`, `{"to":""}`, `{"to":"выдумка"}`, `"строка"`} {
		if rr := postStatus(t, srv, "W-001", body); rr.Code != http.StatusBadRequest {
			t.Errorf("тело %q: код = %d, ожидался 400 (%s)", body, rr.Code, rr.Body.String())
		}
	}
}

func TestPostStatus_getДаёт405(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task/W-001/status", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код = %d, ожидался 405, тело: %s", rr.Code, rr.Body.String())
	}
	var resp errorResponse
	decodeJSON(t, rr, &resp)
	if resp.Error == "" {
		t.Error("405 обязан приходить с JSON-описанием ошибки")
	}
}

func TestPostStatus_подчёркиваниеНормализуется(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	rr := postStatus(t, srv, "W-001", `{"to":"in_progress"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}
	if after := taskFile(t, vaultDir, "w-001.md"); !strings.Contains(after, "status: in-progress") {
		t.Fatalf("статус обязан записываться в каноническом виде:\n%s", after)
	}

	var got apiTask
	decodeJSON(t, rr, &got)
	if got.Status != "in-progress" {
		t.Fatalf("в ответе статус %q, ожидался in-progress", got.Status)
	}
}

// Ключевой тест задачи: один и тот же переход через CLI и через API обязан
// давать побайтово одинаковый файл. Разойдутся — значит правила где-то
// продублированы, и доска с фронтматтером снова начнут спорить.
func TestPostStatus_согласованСCLI(t *testing.T) {
	scenarios := []struct{ id, to string }{
		{"W-001", "in-progress"},
		{"W-001", "done"},
		{"W-003", "cancelled"},
	}
	for _, sc := range scenarios {
		t.Run(sc.id+"->"+sc.to, func(t *testing.T) {
			srv, apiVault := newWriteServer(t)
			cliVault := newWriteVault(t)

			var buf bytes.Buffer
			if err := cli.Set(&buf, cliVault, sc.id, "status", sc.to, "web"); err != nil {
				t.Fatalf("cli.Set: %v", err)
			}
			if rr := postStatus(t, srv, sc.id, `{"to":"`+sc.to+`"}`); rr.Code != http.StatusOK {
				t.Fatalf("API: код = %d, тело: %s", rr.Code, rr.Body.String())
			}

			for name := range writeVaultFiles {
				fromCLI := taskFile(t, cliVault, name)
				fromAPI := taskFile(t, apiVault, name)
				if fromCLI != fromAPI {
					t.Fatalf("%s разошёлся.\nCLI:\n%s\nAPI:\n%s", name, fromCLI, fromAPI)
				}
			}
		})
	}
}
