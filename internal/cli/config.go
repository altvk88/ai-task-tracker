package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Config — файл настроек tt: ровно два значения, которые иначе пришлось бы
// повторять флагами в каждой команде. Третье понадобится — добавится тогда.
type Config struct {
	Vault string `json:"vault,omitempty"`
	Port  int    `json:"port,omitempty"`
}

// Source называет, откуда взялось действующее значение — нужно для tt config show.
type Source string

const (
	SourceFlag    Source = "флаг"
	SourceEnv     Source = "переменная окружения"
	SourceConfig  Source = "файл настроек"
	SourceDefault Source = "значение по умолчанию"
)

// ConfigPath — путь к файлу настроек. TT_CONFIG_PATH переопределяет его
// целиком: тестам он обязателен, чтобы не задеть реальный %APPDATA%/tt.
func ConfigPath() (string, error) {
	if p := os.Getenv("TT_CONFIG_PATH"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	// %APPDATA%/tt на Windows: переживает переустановку самого tt.exe
	// (в отличие от «рядом с бинарником»), а установщику есть куда писать
	// один раз при установке.
	return filepath.Join(dir, "tt", "config.json"), nil
}

// LoadConfig читает файл настроек. Отсутствие файла — не ошибка, а пустая
// конфигурация: команды работают ровно как без файла настроек вовсе.
func LoadConfig() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("файл настроек %s повреждён: %w", path, err)
	}
	return cfg, nil
}

// WriteConfig сохраняет настройки целиком, создавая каталог при необходимости.
func WriteConfig(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// resolveVaultValue ищет значение vault по приоритету флаг → переменная
// окружения → файл настроек, не подтверждая, что это вообще существующий
// каталог с tasks/ — эту проверку делает ResolveVault.
func resolveVaultValue(flagValue string) (value string, source Source, err error) {
	if flagValue != "" {
		return flagValue, SourceFlag, nil
	}
	if v := os.Getenv("TT_VAULT"); v != "" {
		return v, SourceEnv, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return "", "", err
	}
	if cfg.Vault != "" {
		return cfg.Vault, SourceConfig, nil
	}
	return "", SourceDefault, nil
}

// ResolvePort определяет действующий порт: флаг --port → файл настроек →
// встроенный дефолт. flagSet обязателен, потому что у флага --port уже есть
// значение по умолчанию (defaultValue) — без явного признака «флаг реально
// был передан» значение --port 4173 и отсутствие --port неразличимы. Вызывающая
// сторона получает flagSet через fs.Visit после fs.Parse.
func ResolvePort(flagValue int, flagSet bool, defaultValue int) (int, Source, error) {
	if flagSet {
		return flagValue, SourceFlag, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return 0, "", err
	}
	if cfg.Port != 0 {
		return cfg.Port, SourceConfig, nil
	}
	return defaultValue, SourceDefault, nil
}

// ConfigShow печатает действующие vault и port с указанием источника каждого —
// без этого разбираться, откуда взялось «не то» значение, пришлось бы гадать.
func ConfigShow(w io.Writer, vaultFlag string, portFlag int, portFlagSet bool) error {
	vault, vaultSrc, err := resolveVaultValue(vaultFlag)
	if err != nil {
		return err
	}
	if vault == "" {
		fmt.Fprintln(w, "vault: не задан (ни флага, ни TT_VAULT, ни файла настроек)")
	} else {
		fmt.Fprintf(w, "vault: %s (%s)\n", vault, vaultSrc)
	}

	port, portSrc, err := ResolvePort(portFlag, portFlagSet, portFlag)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "port: %d (%s)\n", port, portSrc)

	path, err := ConfigPath()
	if err == nil {
		fmt.Fprintf(w, "файл настроек: %s\n", path)
	}
	return nil
}

// ConfigSet пишет настройки неинтерактивно — ровно для того, чтобы установщик
// мог задать vault и порт одной командой. Меняет только переданные поля,
// остальные сохраняет как есть.
func ConfigSet(w io.Writer, vault string, vaultSet bool, port int, portSet bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if vaultSet {
		cfg.Vault = vault
	}
	if portSet {
		if port <= 0 {
			return fmt.Errorf("port должен быть положительным, получено %d", port)
		}
		cfg.Port = port
	}
	if err := WriteConfig(cfg); err != nil {
		return err
	}
	path, _ := ConfigPath()
	fmt.Fprintf(w, "настройки сохранены: %s\n", path)
	return nil
}
