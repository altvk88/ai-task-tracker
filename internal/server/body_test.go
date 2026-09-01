package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postBody(t *testing.T, srv *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/task/"+id+"/body", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

// frontmatterOf вырезает фронтматтер (оба фенса включительно) — тем же
// способом, каким SetBody его обязана оставить нетронутым.
func frontmatterOf(t *testing.T, text string) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t\r") == "---" {
			return strings.Join(lines[:i+1], "\n")
		}
	}
	t.Fatal("не нашёл закрывающий фенс")
	return ""
}

func TestSetBody_записываетТелоИНеТрогаетФронтматтер(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	before := taskFile(t, vaultDir, "w-001.md")
	v := currentVersion(t, srv, "W-001")

	newBody := `{"body":"## Другое тело\n\nПравка с панели.\n","baseVersion":"` + v + `"}`
	rr := postBody(t, srv, "W-001", newBody)
	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	after := taskFile(t, vaultDir, "w-001.md")
	if frontmatterOf(t, after) != frontmatterOf(t, before) {
		t.Fatalf("фронтматтер изменился:\n%s", after)
	}
	if !strings.Contains(after, "Правка с панели.") {
		t.Fatalf("тело не записано:\n%s", after)
	}
}

func TestSetBody_безBaseVersion400(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := postBody(t, srv, "W-001", `{"body":"текст"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("код = %d, ожидался 400, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestSetBody_устаревшаяВерсияДаётКонфликт(t *testing.T) {
	srv, vaultDir := newWriteServer(t)
	stale := currentVersion(t, srv, "W-001")

	if rr := postField(t, srv, "W-001", `{"key":"priority","value":"low","baseVersion":"`+stale+`"}`); rr.Code != http.StatusOK {
		t.Fatalf("подготовка: код = %d, тело: %s", rr.Code, rr.Body.String())
	}
	before := taskFile(t, vaultDir, "w-001.md")

	rr := postBody(t, srv, "W-001", `{"body":"новое тело","baseVersion":"`+stale+`"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("код = %d, ожидался 409, тело: %s", rr.Code, rr.Body.String())
	}
	if after := taskFile(t, vaultDir, "w-001.md"); after != before {
		t.Fatalf("конфликт по версии не должен менять файл:\n%s", after)
	}
}

func TestSetBody_ненайденнаяТаска404(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := postBody(t, srv, "W-404", `{"body":"текст","baseVersion":"любая"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("код = %d, ожидался 404, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestSetBody_кривоеТело400(t *testing.T) {
	srv, _ := newWriteServer(t)
	for _, body := range []string{``, `{`, `"строка"`} {
		if rr := postBody(t, srv, "W-001", body); rr.Code != http.StatusBadRequest {
			t.Errorf("тело %q: код = %d, ожидался 400 (%s)", body, rr.Code, rr.Body.String())
		}
	}
}

func TestSetBody_getДаёт405(t *testing.T) {
	srv, _ := newWriteServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/task/W-001/body", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код = %d, ожидался 405, тело: %s", rr.Code, rr.Body.String())
	}
}
