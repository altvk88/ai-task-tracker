package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/model"
)

// TestLiveVault — постоянный смоук-тест на реальном vault. Включается
// переменной окружения TT_SMOKE_VAULT (без неё — Skip), т.к. живой vault
// не часть репозитория tt и на CI его нет.
//
// Тест окупился один раз, найдя, что модель отвергает 44% реальных тасок
// (verify и claim скаляром вместо ожидаемых форм). Остаётся навсегда, чтобы
// не откатиться обратно к строгой модели незаметно.
//
// ВАЖНО: vault только на чтение. Для проверки SetField таска копируется в
// t.TempDir(), правится копия.
func TestLiveVault(t *testing.T) {
	root := os.Getenv("TT_SMOKE_VAULT")
	if root == "" {
		t.Skip("TT_SMOKE_VAULT не задан, смоук на живом vault пропущен")
	}
	tasksDir := filepath.Join(root, "tasks")

	dirs, err := os.ReadDir(tasksDir)
	if err != nil {
		t.Fatalf("чтение %s: %v", tasksDir, err)
	}

	var total, broken int
	var brokenPaths []string

	for _, d := range dirs {
		if !d.IsDir() || strings.HasPrefix(d.Name(), "_") {
			continue
		}
		projDir := filepath.Join(tasksDir, d.Name())
		files, err := os.ReadDir(projDir)
		if err != nil {
			t.Fatalf("чтение %s: %v", projDir, err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			path := filepath.Join(projDir, f.Name())
			total++

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("чтение %s: %v", path, err)
			}
			task, err := Parse(raw)
			if err != nil {
				broken++
				if len(brokenPaths) < 10 {
					brokenPaths = append(brokenPaths, path+": "+err.Error())
				}
				continue
			}

			checkSetFieldRoundTrip(t, path, string(raw), task)
		}
	}

	t.Logf("всего тасок: %d, неразбираемых: %d", total, broken)
	if broken > 0 {
		t.Logf("первые неразбираемые:\n%s", strings.Join(brokenPaths, "\n"))
	}
	// Порог не 12 (только дубли ключей), а с запасом: помимо дублей ключей
	// в vault нашлись ещё три вида порчи данных, которые задача не просила
	// чинить и которые по тому же принципу ("дубли не лечим — это порча
	// данных") остаются ошибками разбора:
	//   - result: как вложенная YAML-мапа вместо строки, ~7 тасок intake;
	//   - неэкранированные кавычки внутри квотированного скаляра
	//     (например result: "... position="center" ..."), ~15 тасок;
	//   - незакрытый фронтматтер там, где tt doctor --fix его не восстанавливает:
	//     3 таски, у которых после вставки фенса остаётся дубль ключа claim
	//     или вовсе потерян заголовок ## Log.
	// Незакрытый фронтматтер в остальных 34 тасках чинится `tt doctor --fix`
	// (см. vault.RestoreFence), поэтому фактическое число упало с 65 до 31
	// на 2026-08-31. Порог — 40: запас на несколько новых битых тасок, но
	// заметно ниже прежних 80, чтобы возврат целого класса порчи всплыл.
	const maxBroken = 40
	if broken > maxBroken {
		t.Errorf("неразбираемых тасок %d, порог %d — модель снова отвергает больше, чем только дубли ключей", broken, maxBroken)
	}
}

// checkSetFieldRoundTrip копирует таску во временный каталог, меняет status
// на заведомо другое значение и проверяет, что изменилась ровно одна строка,
// файл по-прежнему разбирается, а ID и Title не поплыли.
func checkSetFieldRoundTrip(t *testing.T, path, before string, task model.Task) {
	t.Helper()

	newStatus := "ready"
	if task.Status == "ready" {
		newStatus = "backlog"
	}

	tmp := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := os.WriteFile(tmp, []byte(before), 0o644); err != nil {
		t.Fatalf("копирование %s: %v", path, err)
	}

	if err := SetField(tmp, "status", newStatus); err != nil {
		t.Errorf("%s: SetField: %v", path, err)
		return
	}

	after, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("чтение копии %s: %v", tmp, err)
	}

	beforeLines := strings.Split(strings.ReplaceAll(before, "\r\n", "\n"), "\n")
	afterLines := strings.Split(strings.ReplaceAll(string(after), "\r\n", "\n"), "\n")
	if len(beforeLines) != len(afterLines) {
		t.Errorf("%s: SetField изменил число строк: было %d, стало %d", path, len(beforeLines), len(afterLines))
		return
	}
	diff := 0
	for i := range beforeLines {
		if beforeLines[i] != afterLines[i] {
			diff++
		}
	}
	if diff != 1 {
		t.Errorf("%s: SetField изменил %d строк, ожидалась ровно 1", path, diff)
	}

	got, err := Parse(after)
	if err != nil {
		t.Errorf("%s: после SetField файл перестал разбираться: %v", path, err)
		return
	}
	if got.ID != task.ID {
		t.Errorf("%s: ID поплыл: было %q, стало %q", path, task.ID, got.ID)
	}
	if got.Title != task.Title {
		t.Errorf("%s: Title поплыл: было %q, стало %q", path, task.Title, got.Title)
	}
}
