// Package cli — тонкая обёртка над model и vault. Никакой логики флоу здесь нет.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveVault определяет путь к vault: флаг --vault, иначе TT_VAULT.
// Пути по умолчанию нет намеренно: молча писать в чужой каталог хуже,
// чем потребовать один явный флаг.
func ResolveVault(flagValue string) (string, error) {
	candidate := flagValue
	if candidate == "" {
		candidate = os.Getenv("TT_VAULT")
	}
	if candidate == "" {
		return "", fmt.Errorf("не задан путь к vault: укажи --vault <путь> или переменную TT_VAULT")
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(filepath.Join(abs, "tasks"))
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s не похож на vault: нет каталога tasks", abs)
	}
	return abs, nil
}

// SchemaPath — путь к общему контракту правил внутри vault.
func SchemaPath(vaultDir string) string {
	return filepath.Join(vaultDir, ".task-tracker", "schema.json")
}
