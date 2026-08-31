package index

import (
	"os"
	"testing"
	"time"

	"github.com/alkulagin-creator/tt/internal/vault"
)

// TestLiveVault_смоук — строит индекс на живом vault (только чтение,
// TT_SMOKE_VAULT) и сверяет число тасок с vault.Scan напрямую, а также
// прикидывает время построения. По образцу internal/vault/livevault_test.go.
func TestLiveVault_смоук(t *testing.T) {
	root := os.Getenv("TT_SMOKE_VAULT")
	if root == "" {
		t.Skip("TT_SMOKE_VAULT не задан, смоук на живом vault пропущен")
	}

	want, err := vault.Scan(root)
	if err != nil {
		t.Fatalf("vault.Scan: %v", err)
	}

	start := time.Now()
	ix, err := New(root)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got := ix.Snapshot()
	if len(got) != len(want) {
		t.Errorf("в индексе %d тасок, vault.Scan нашёл %d", len(got), len(want))
	}

	t.Logf("построение индекса: %d тасок за %s", len(got), elapsed)
}
