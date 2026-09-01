// Package cli — тонкая обёртка над model и vault. Никакой логики флоу здесь нет.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// ResolveVault определяет путь к vault: флаг --vault, иначе TT_VAULT, иначе
// файл настроек (см. config.go). Пути по умолчанию нет намеренно: молча
// писать в чужой каталог хуже, чем потребовать один явный флаг или запись
// в файле настроек — оба варианта видны и осознанны.
func ResolveVault(flagValue string) (string, error) {
	candidate, _, err := resolveVaultValue(flagValue)
	if err != nil {
		return "", err
	}
	if candidate == "" {
		return "", fmt.Errorf("не задан путь к vault: укажи --vault <путь>, переменную TT_VAULT или запиши его в файл настроек (tt config set --vault <путь>)")
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
	return taskop.SchemaPath(vaultDir)
}
