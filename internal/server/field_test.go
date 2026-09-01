package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postField — как postStatus, но на /field: тот же локальный RemoteAddr,
// авторизация здесь не по теме теста.
func postField(t *testing.T, srv *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/task/"+id+"/field", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

// currentVersion читает актуальную версию таски через GET /api/task/{id} —
// так же, как это делал бы настоящий клиент перед правкой.
func currentVersion(t *testing.T, srv *Server, id string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task/"+id, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/task/%s: код = %d, тело: %s", id, rr.Code, rr.Body.String())
	}
	var detail taskDetail
	decodeJSON(t, rr, &detail)
	if detail.Version == "" {
		t.Fatal("снимок задачи не содержит version")
	}
	return detail.Version
}

func TestSetField_меняетПроизвольноеПоле(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	v := currentVersion(t, srv, "W-001")

	rr := postField(t, srv, "W-001", `{"key":"priority","value":"low","baseVersion":"`+v+`"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}
	var got apiTask
	decodeJSON(t, rr, &got)
	if got.Priority != "low" {
		t.Fatalf("Priority = %q, ожидался low", got.Priority)
	}
	if !strings.Contains(taskFile(t, vaultDir, "w-001.md"), "priority: low") {
		t.Fatal("поле не записано в файл")
	}
}

func TestSetField_незнакомыйКлючОтклоняется(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	before := taskFile(t, vaultDir, "w-001.md")
	v := currentVersion(t, srv, "W-001")

	rr := postField(t, srv, "W-001", `{"key":"выдумка","value":"x","baseVersion":"`+v+`"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("код = %d, ожидался 400, тело: %s", rr.Code, rr.Body.String())
	}
	if after := taskFile(t, vaultDir, "w-001.md"); after != before {
		t.Fatalf("отклонённое поле изменило файл:\n%s", after)
	}
}

func TestSetField_статусОтклоняетсяССылкойНаSpecialРоут(t *testing.T) {
	srv, _ := newWriteServer(t)
	v := currentVersion(t, srv, "W-001")
	rr := postField(t, srv, "W-001", `{"key":"status","value":"hold","baseVersion":"`+v+`"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("код = %d, ожидался 400, тело: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/status") {
		t.Errorf("ошибка обязана указывать на /status: %s", rr.Body.String())
	}
}

func TestSetField_безBaseVersion400(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := postField(t, srv, "W-001", `{"key":"priority","value":"low"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("код = %d, ожидался 400, тело: %s", rr.Code, rr.Body.String())
	}
}

// Ключевой тест задачи: устаревшая версия обязана давать конфликт, а не
// тихо перезаписывать чужую правку.
func TestSetField_устаревшаяВерсияДаётКонфликт(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	stale := currentVersion(t, srv, "W-001")

	// Чужая правка между чтением клиента и его записью — например, через CLI.
	if rr := postField(t, srv, "W-001", `{"key":"priority","value":"medium","baseVersion":"`+stale+`"}`); rr.Code != http.StatusOK {
		t.Fatalf("подготовка: код = %d, тело: %s", rr.Code, rr.Body.String())
	}
	before := taskFile(t, vaultDir, "w-001.md")

	rr := postField(t, srv, "W-001", `{"key":"priority","value":"low","baseVersion":"`+stale+`"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("код = %d, ожидался 409, тело: %s", rr.Code, rr.Body.String())
	}
	if after := taskFile(t, vaultDir, "w-001.md"); after != before {
		t.Fatalf("конфликт по версии не должен менять файл:\n%s", after)
	}
}

func TestSetField_ненайденнаяТаска404(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := postField(t, srv, "W-404", `{"key":"priority","value":"low","baseVersion":"любая"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("код = %d, ожидался 404, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestSetField_getДаёт405(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task/W-001/field", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код = %d, ожидался 405, тело: %s", rr.Code, rr.Body.String())
	}
}
