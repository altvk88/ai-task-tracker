package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alkulagin-creator/tt/internal/index"
)

// TestLiveVault_снимок — строит индекс на живом vault (только чтение,
// TT_SMOKE_VAULT), поднимает сервер через httptest (без реального порта) и
// сверяет число тасок в JSON-снимке с index.Snapshot(). Заодно логирует
// размер ответа в КБ — это то, что поедет в браузер на каждой загрузке доски.
func TestLiveVault_снимок(t *testing.T) {
	root := os.Getenv("TT_SMOKE_VAULT")
	if root == "" {
		t.Skip("TT_SMOKE_VAULT не задан, смоук на живом vault пропущен")
	}

	ix, err := index.New(root)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	want := ix.Snapshot()

	srv := New(ix, root, Options{})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("код = %d, тело: %s", rr.Code, rr.Body.String())
	}

	var resp snapshotResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не разбирается как JSON: %v", err)
	}

	if len(resp.Tasks) != len(want) {
		t.Errorf("в JSON-снимке %d тасок, index.Snapshot() отдал %d", len(resp.Tasks), len(want))
	}

	kb := float64(rr.Body.Len()) / 1024
	t.Logf("живой vault: %d тасок, JSON-снимок %.1f КБ", len(resp.Tasks), kb)
}
