package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveVault(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("флаг важнее переменной окружения", func(t *testing.T) {
		t.Setenv("TT_VAULT", "C:/несуществующий")
		got, err := ResolveVault(root)
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if got != root {
			t.Errorf("получено %q, ожидалось %q", got, root)
		}
	})

	t.Run("без флага берётся TT_VAULT", func(t *testing.T) {
		t.Setenv("TT_VAULT", root)
		got, err := ResolveVault("")
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if got != root {
			t.Errorf("получено %q, ожидалось %q", got, root)
		}
	})

	t.Run("нет ни флага, ни переменной — внятная ошибка", func(t *testing.T) {
		t.Setenv("TT_VAULT", "")
		if _, err := ResolveVault(""); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("каталог без tasks отклоняется", func(t *testing.T) {
		if _, err := ResolveVault(t.TempDir()); err == nil {
			t.Fatal("каталог без tasks/ не является vault")
		}
	})
}
