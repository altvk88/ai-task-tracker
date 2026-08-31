package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alkulagin-creator/tt/internal/index"
	"github.com/alkulagin-creator/tt/internal/model"
)

// --- Юнит-тесты serveSSE: без сети, без сна, полностью детерминированные ---

func TestServeSSE_событиеИзменения(t *testing.T) {
	schema, err := model.DefaultSchema()
	if err != nil {
		t.Fatalf("DefaultSchema: %v", err)
	}

	ch := make(chan index.Change, 1)
	task := model.Task{ID: "DEMO-1", Status: "ready", Project: "demo", Title: "т"}
	ch <- index.Change{ID: "DEMO-1", Kind: index.Updated, Task: task}
	close(ch)

	rr := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	serveSSE(ctx, rr, ch, schema, time.Hour)

	body := rr.Body.String()
	if !strings.Contains(body, "event: change") {
		t.Fatalf("не найдено событие change: %q", body)
	}
	if !strings.Contains(body, `"id":"DEMO-1"`) || !strings.Contains(body, `"kind":"updated"`) {
		t.Fatalf("тело события неполное: %q", body)
	}
	if !strings.Contains(body, `"status":"ready"`) {
		t.Fatalf("статус таски не попал в событие: %q", body)
	}
}

func TestServeSSE_закрытиеКаналаДаётResync(t *testing.T) {
	ch := make(chan index.Change)
	close(ch)

	rr := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	serveSSE(ctx, rr, ch, nil, time.Hour)

	body := rr.Body.String()
	if !strings.Contains(body, "event: resync") {
		t.Fatalf("ожидалось событие resync, получено: %q", body)
	}
}

func TestServeSSE_пульсНастраиваемый(t *testing.T) {
	ch := make(chan index.Change)
	rr := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	serveSSE(ctx, rr, ch, nil, 20*time.Millisecond)

	body := rr.Body.String()
	if !strings.Contains(body, ": heartbeat") {
		t.Fatalf("пульс не пришёл за отведённое время: %q", body)
	}
}

// --- Интеграционные тесты: настоящий поток через httptest.NewServer ---

// sseClient подключается к /api/events на тестовом сервере и построчно читает
// события. httptest.NewServer нужен вместо ResponseRecorder именно здесь:
// клиенту нужно читать тело конкурентно с тем, как сервер его пишет, а
// ResponseRecorder этого не умеет.
type sseClient struct {
	t      *testing.T
	resp   *http.Response
	reader *bufio.Reader
	cancel context.CancelFunc
}

func connectSSE(t *testing.T, url string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("Do: %v", err)
	}
	return &sseClient{t: t, resp: resp, reader: bufio.NewReader(resp.Body), cancel: cancel}
}

func (c *sseClient) close() {
	c.cancel()
	c.resp.Body.Close()
}

// waitForLine читает строки, пока одна из них не содержит substr, либо не
// истечёт timeout. Чтение блокирующее, поэтому таймаут реализован отдельной
// горутиной, закрывающей тело ответа и тем самым отменяющей Read.
func (c *sseClient) waitForLine(substr string, timeout time.Duration) string {
	c.t.Helper()
	type result struct {
		line string
		err  error
	}
	lines := make(chan result)
	go func() {
		for {
			line, err := c.reader.ReadString('\n')
			lines <- result{line, err}
			if err != nil {
				return
			}
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case res := <-lines:
			if res.err != nil {
				c.t.Fatalf("чтение потока прервалось: %v", res.err)
			}
			if strings.Contains(res.line, substr) {
				return res.line
			}
		case <-timer.C:
			c.t.Fatalf("не дождались строки, содержащей %q", substr)
		}
	}
}

func TestEvents_клиентПолучаетИзменение(t *testing.T) {
	srv, vaultDir := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := connectSSE(t, ts.URL+"/api/events")
	defer client.close()

	path := filepath.Join(vaultDir, "tasks", "demo", "demo-001.md")
	updated := "---\nid: DEMO-001\ntitle: Первая таска\nstatus: in-progress\nproject: demo\npriority: high\ncreated: 2026-08-01\n---\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := srv.ix.Apply(path); !ok {
		t.Fatal("Apply не увидел изменения")
	}

	line := client.waitForLine(`"id":"DEMO-001"`, 3*time.Second)
	if !strings.Contains(line, `"status":"in-progress"`) {
		t.Errorf("в событии нет нового статуса: %q", line)
	}
}

func TestEvents_двумКлиентамПриходитОдноИТоЖе(t *testing.T) {
	srv, vaultDir := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	a := connectSSE(t, ts.URL+"/api/events")
	defer a.close()
	b := connectSSE(t, ts.URL+"/api/events")
	defer b.close()

	path := filepath.Join(vaultDir, "tasks", "demo", "demo-002.md")
	updated := "---\nid: DEMO-002\ntitle: Вторая таска\nstatus: done\nproject: demo\npriority: low\ncreated: 2026-08-02\n---\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := srv.ix.Apply(path); !ok {
		t.Fatal("Apply не увидел изменения")
	}

	a.waitForLine(`"id":"DEMO-002"`, 3*time.Second)
	b.waitForLine(`"id":"DEMO-002"`, 3*time.Second)
}

func TestEvents_отключениеОдногоНеЛомаетВторого(t *testing.T) {
	srv, vaultDir := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	a := connectSSE(t, ts.URL+"/api/events")
	b := connectSSE(t, ts.URL+"/api/events")
	defer b.close()

	a.close()

	path := filepath.Join(vaultDir, "tasks", "demo", "demo-002.md")
	updated := "---\nid: DEMO-002\ntitle: Вторая таска\nstatus: done\nproject: demo\npriority: low\ncreated: 2026-08-02\n---\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := srv.ix.Apply(path); !ok {
		t.Fatal("Apply не увидел изменения")
	}

	b.waitForLine(`"id":"DEMO-002"`, 3*time.Second)
}

func TestEvents_заголовкиОтвета(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := connectSSE(t, ts.URL+"/api/events")
	defer client.close()

	resp := client.resp
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, ожидался text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, ожидался no-cache", cc)
	}
	if conn := resp.Header.Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q, ожидался keep-alive", conn)
	}
}

func TestEvents_пульсПриходитВЖивомСоединении(t *testing.T) {
	vaultDir := newTestVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	srv := New(ix, vaultDir, Options{HeartbeatInterval: 20 * time.Millisecond})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := connectSSE(t, ts.URL+"/api/events")
	defer client.close()

	client.waitForLine(": heartbeat", 2*time.Second)
}

// TestEvents_отставшийПодписчикПолучаетResync проверяет реакцию на закрытие
// подписки не через настоящую сетевую задержку (TCP-окно переполнить
// таймингонезависимо не выйдет — see flaky history), а напрямую: держим
// соединение открытым, но не читаем из него ни байта, поэтому запись
// хендлера в сокет рано или поздно заблокируется на заполненном буфере ОС;
// пока хендлер заблокирован на Write, буфер канала подписчика (64 события)
// переполняется реальными изменениями индекса, и index закрывает канал.
// Чтобы не зависеть от точного размера ОС-буфера, читерски уменьшаем окно
// приёма сокета до минимума через SetReadBuffer.
func TestEvents_отставшийПодписчикПолучаетResync(t *testing.T) {
	vaultDir := newTestVault(t)
	ix, err := index.New(vaultDir)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	srv := New(ix, vaultDir, Options{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetReadBuffer(1)
	}
	if _, err := fmt.Fprintf(conn, "GET /api/events HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", addr); err != nil {
		t.Fatalf("запрос: %v", err)
	}

	// Заливаем изменения, ничего не читая из соединения: index.Apply не
	// блокируется на медленном подписчике (broadcast — неблокирующий select),
	// поэтому цикл отрабатывает почти мгновенно, а вот хендлер на другом
	// конце рано или поздно застревает в Write на заполненном окне сокета —
	// и именно тогда канал подписчика (буфер 64) переполняется и index его
	// закрывает.
	path := filepath.Join(vaultDir, "tasks", "demo", "demo-002.md")
	statuses := []string{"ready", "in-progress"}
	for i := 0; i < 2000; i++ {
		st := statuses[i%2]
		body := "---\nid: DEMO-002\ntitle: t\nstatus: " + st + "\nproject: demo\n---\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		ix.Apply(path)
	}

	// Теперь начинаем читать — включая то, что сервер успел отправить, и то,
	// что "разморозится" после этого.
	reader := bufio.NewReader(conn)
	deadline := time.Now().Add(10 * time.Second)
	conn.SetReadDeadline(deadline)
	var seenResync bool
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if strings.Contains(line, "event: resync") {
			seenResync = true
			break
		}
		if err != nil {
			break
		}
	}
	if !seenResync {
		t.Fatal("не дождались события resync для отставшего подписчика")
	}
}

func TestEvents_горутиныНеТекутПослеОтключения(t *testing.T) {
	srv, vaultDir := newTestServer(t)
	_ = vaultDir
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	before := runtime.NumGoroutine()

	clients := make([]*sseClient, 5)
	for i := range clients {
		clients[i] = connectSSE(t, ts.URL+"/api/events")
	}
	for _, c := range clients {
		c.close()
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		runtime.Gosched()
		if runtime.NumGoroutine() <= before+2 { // допуск на планировщик/GC
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("похоже на утечку горутин: было %d, стало %d", before, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
