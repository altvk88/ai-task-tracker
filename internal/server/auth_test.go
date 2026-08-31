package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkulagin-creator/tt/internal/index"
)

// newAuthServer поднимает сервер с фиксированным токеном на vault из
// newWriteVault: содержимое тасок для проверки авторизации не важно, но
// нужна хотя бы одна существующая (W-001), чтобы не путать 401 с 404.
func newAuthServer(t *testing.T, token string) *Server {
	t.Helper()
	vaultDir := newWriteVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	return New(ix, vaultDir, Options{Token: token})
}

func doWithAddr(srv *Server, method, path, remoteAddr string, headers map[string]string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestAuth_loopbackPostБезТокенаПроходит(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "127.0.0.1:5555", nil)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("loopback без токена получил 401, тело: %s", rr.Body.String())
	}
}

func TestAuth_нелupbackGetБезТокенаПроходит(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodGet, "/api/snapshot", "203.0.113.5:5555", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, ожидался 200, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestAuth_нелupbackPostБезТокена401(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "203.0.113.5:5555", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("код = %d, ожидался 401, тело: %s", rr.Code, rr.Body.String())
	}
	var resp errorResponse
	decodeJSON(t, rr, &resp)
	if resp.Error == "" {
		t.Error("401 обязан приходить с JSON-описанием ошибки")
	}
}

func TestAuth_нелupbackPostСВернымТокеномВЗаголовке(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "203.0.113.5:5555",
		map[string]string{"Authorization": "Bearer секрет", "Content-Type": "application/json"})
	// Тело запроса пустое — важно только то, что дальше запрос не отклонён
	// авторизацией (401 быть не должно; 400 из-за пустого тела допустим).
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("верный токен в заголовке отклонён: %s", rr.Body.String())
	}
}

func TestAuth_нелupbackPostСВернымТокеномВПараметре(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status?token=секрет", "203.0.113.5:5555", nil)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("верный токен в параметре отклонён: %s", rr.Body.String())
	}
}

func TestAuth_нелupbackPostСНеверннымТокеном401(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "203.0.113.5:5555",
		map[string]string{"Authorization": "Bearer неверный"})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("код = %d, ожидался 401, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestAuth_localhostIPv6СчитаетсяLoopback(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "[::1]:5555", nil)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("::1 не распознан как loopback: %s", rr.Body.String())
	}
}

func TestAuth_адресИз127Сети0Loopback(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "127.42.1.7:5555", nil)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("127.42.1.7 не распознан как loopback: %s", rr.Body.String())
	}
}

func TestAuth_подделанныйXForwardedForНеДаётДоступа(t *testing.T) {
	srv := newAuthServer(t, "секрет")
	rr := doWithAddr(srv, http.MethodPost, "/api/task/W-001/status", "203.0.113.5:5555",
		map[string]string{"X-Forwarded-For": "127.0.0.1"})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("подделанный X-Forwarded-For дал доступ: код = %d, тело: %s", rr.Code, rr.Body.String())
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:80":   true,
		"127.42.1.7:80":  true,
		"[::1]:80":       true,
		"203.0.113.5:80": false,
		"10.0.0.5:80":    false,
		"не-адрес":       false,
		"":               false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, ожидалось %v", addr, got, want)
		}
	}
}

func TestGenerateToken_генерируетНепустойИРазныйКаждыйРаз(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("токен пустой")
	}
	if a == b {
		t.Fatal("два вызова дали одинаковый токен")
	}
	if len(a) < 32 {
		t.Fatalf("токен слишком короткий: %d символов", len(a))
	}
}
