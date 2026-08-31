package index

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// TestLiveVault_слежение — поднимает Watch на живом vault (только чтение,
// TT_SMOKE_VAULT) на несколько секунд и проверяет, что он стартует без
// ошибок и корректно останавливается по отмене контекста. В живой vault
// ничего не пишется.
func TestLiveVault_слежение(t *testing.T) {
	root := os.Getenv("TT_SMOKE_VAULT")
	if root == "" {
		t.Skip("TT_SMOKE_VAULT не задан, смоук на живом vault пропущен")
	}

	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	dirs := countTaskDirs(t, root)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- Watch(ctx, ix, root) }()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Watch завершился с ошибкой: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch не остановился по истечении контекста")
	}

	t.Logf("слежение поднялось на %d каталогов живого vault", dirs)
}

// countTaskDirs считает каталоги под <root>/tasks, на которые подпишется
// Watch (та же логика пропуска "_", что и в vault.Scan/addDirRecursive) —
// только для отчёта в логе теста.
func countTaskDirs(t *testing.T, root string) int {
	t.Helper()
	tasksDir := filepath.Join(root, "tasks")
	n := 0
	err := filepath.WalkDir(tasksDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != tasksDir && strings.HasPrefix(d.Name(), "_") {
			return fs.SkipDir
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatalf("подсчёт каталогов: %v", err)
	}
	return n
}
