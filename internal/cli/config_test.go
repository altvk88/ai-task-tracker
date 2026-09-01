package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withConfigPath направляет ConfigPath на файл внутри t.TempDir(), чтобы тест
// не касался реального %APPDATA%/tt пользователя.
func withConfigPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("TT_CONFIG_PATH", path)
	return path
}

func TestLoadConfigMissingFileIsNotError(t *testing.T) {
	withConfigPath(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("отсутствие файла настроек не должно быть ошибкой: %v", err)
	}
	if cfg.Vault != "" || cfg.Port != 0 {
		t.Fatalf("ожидалась пустая конфигурация, получено %+v", cfg)
	}
}

func TestLoadConfigBrokenJSONNamesPath(t *testing.T) {
	path := withConfigPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{не json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("ожидалась ошибка на битом JSON")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("ошибка должна называть путь %q, получено: %v", path, err)
	}
}

func TestWriteConfigRoundTrip(t *testing.T) {
	path := withConfigPath(t)
	want := Config{Vault: "D:/vault", Port: 4180}
	if err := WriteConfig(want); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл настроек не создан: %v", err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("получено %+v, ожидалось %+v", got, want)
	}
}

func TestResolveVaultLevels(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	otherDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(otherDir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("только файл настроек", func(t *testing.T) {
		withConfigPath(t)
		t.Setenv("TT_VAULT", "")
		if err := WriteConfig(Config{Vault: vaultDir}); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveVault("")
		if err != nil {
			t.Fatal(err)
		}
		if got != vaultDir {
			t.Errorf("получено %q, ожидалось %q", got, vaultDir)
		}
	})

	t.Run("переменная окружения важнее файла настроек", func(t *testing.T) {
		withConfigPath(t)
		if err := WriteConfig(Config{Vault: otherDir}); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TT_VAULT", vaultDir)
		got, err := ResolveVault("")
		if err != nil {
			t.Fatal(err)
		}
		if got != vaultDir {
			t.Errorf("переменная окружения должна была победить файл настроек")
		}
	})

	t.Run("флаг важнее файла настроек и переменной", func(t *testing.T) {
		withConfigPath(t)
		if err := WriteConfig(Config{Vault: otherDir}); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TT_VAULT", otherDir)
		got, err := ResolveVault(vaultDir)
		if err != nil {
			t.Fatal(err)
		}
		if got != vaultDir {
			t.Errorf("флаг должен был победить и файл, и переменную")
		}
	})

	t.Run("нет ни флага, ни переменной, ни файла — прежняя ошибка", func(t *testing.T) {
		withConfigPath(t)
		t.Setenv("TT_VAULT", "")
		if _, err := ResolveVault(""); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("битый файл настроек — ошибка с путём даже без флага и переменной", func(t *testing.T) {
		path := withConfigPath(t)
		t.Setenv("TT_VAULT", "")
		if err := os.WriteFile(path, []byte("{битый"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ResolveVault("")
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("ожидалась ошибка с путём %q, получено: %v", path, err)
		}
	})
}

func TestResolvePortLevels(t *testing.T) {
	const def = 4173

	t.Run("флаг важнее файла настроек", func(t *testing.T) {
		withConfigPath(t)
		if err := WriteConfig(Config{Port: 9000}); err != nil {
			t.Fatal(err)
		}
		got, _, err := ResolvePort(def, true, def)
		if err != nil {
			t.Fatal(err)
		}
		if got != def {
			t.Errorf("флаг --port %d должен был победить файл настроек, получено %d", def, got)
		}
	})

	t.Run("флаг --port ровно с дефолтным значением всё равно побеждает файл", func(t *testing.T) {
		// Ключевая проверка неразличимости: явно передан --port 4173, что
		// совпадает со встроенным дефолтом. flagSet=true обязан сохранить флаг.
		withConfigPath(t)
		if err := WriteConfig(Config{Port: 9000}); err != nil {
			t.Fatal(err)
		}
		got, src, err := ResolvePort(def, true, def)
		if err != nil {
			t.Fatal(err)
		}
		if got != def || src != SourceFlag {
			t.Errorf("получено %d/%s, ожидалось %d/%s", got, src, def, SourceFlag)
		}
	})

	t.Run("флаг не задан — берётся файл настроек", func(t *testing.T) {
		withConfigPath(t)
		if err := WriteConfig(Config{Port: 9000}); err != nil {
			t.Fatal(err)
		}
		got, src, err := ResolvePort(def, false, def)
		if err != nil {
			t.Fatal(err)
		}
		if got != 9000 || src != SourceConfig {
			t.Errorf("получено %d/%s, ожидалось 9000/%s", got, src, SourceConfig)
		}
	})

	t.Run("ни флага, ни файла — встроенный дефолт", func(t *testing.T) {
		withConfigPath(t)
		got, src, err := ResolvePort(def, false, def)
		if err != nil {
			t.Fatal(err)
		}
		if got != def || src != SourceDefault {
			t.Errorf("получено %d/%s, ожидалось %d/%s", got, src, def, SourceDefault)
		}
	})

	t.Run("битый файл настроек — ошибка с путём", func(t *testing.T) {
		path := withConfigPath(t)
		if err := os.WriteFile(path, []byte("{битый"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := ResolvePort(def, false, def)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("ожидалась ошибка с путём %q, получено: %v", path, err)
		}
	})
}

func TestConfigShowNamesSources(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	withConfigPath(t)
	t.Setenv("TT_VAULT", "")
	if err := WriteConfig(Config{Vault: vaultDir, Port: 9000}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := ConfigShow(&buf, "", 4173, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, vaultDir) || !strings.Contains(out, string(SourceConfig)) {
		t.Errorf("вывод должен называть vault и его источник (файл настроек): %s", out)
	}
	if !strings.Contains(out, "9000") {
		t.Errorf("вывод должен называть порт из файла настроек: %s", out)
	}
}

func TestConfigSetWritesOnlyGivenFields(t *testing.T) {
	path := withConfigPath(t)
	_ = path
	vaultDir := t.TempDir()

	var buf bytes.Buffer
	if err := ConfigSet(&buf, vaultDir, true, 0, false); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Vault != vaultDir || got.Port != 0 {
		t.Fatalf("получено %+v", got)
	}

	buf.Reset()
	if err := ConfigSet(&buf, "", false, 9191, true); err != nil {
		t.Fatal(err)
	}
	got, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Vault != vaultDir || got.Port != 9191 {
		t.Fatalf("второй ConfigSet должен был сохранить vault и добавить port, получено %+v", got)
	}
}

func TestLoadConfigNeverTouchesRealUserProfile(t *testing.T) {
	// Без TT_CONFIG_PATH ConfigPath уходит в os.UserConfigDir() — реальный
	// профиль пользователя. Тест фиксирует, что переменная окружения его
	// подменяет, и что во всех остальных тестах файла она обязательна.
	path := withConfigPath(t)
	got, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("TT_CONFIG_PATH должен полностью определять путь, получено %q", got)
	}
}
