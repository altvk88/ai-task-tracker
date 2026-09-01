package taskop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

func TestScaffoldEmptyDirCreatesEverything(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myvault")
	res, err := Scaffold(root, ScaffoldOptions{})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	want := []string{"tasks/", "templates/task-template.md", ".task-tracker/schema.json", "projects/myvault.md"}
	if len(res.Created) != len(want) {
		t.Fatalf("Created = %v, ожидалось %v", res.Created, want)
	}
	for i, w := range want {
		if res.Created[i] != w {
			t.Errorf("Created[%d] = %q, ожидалось %q", i, res.Created[i], w)
		}
	}
	if len(res.Skipped) != 0 {
		t.Errorf("Skipped = %v, ожидалось пусто на пустом каталоге", res.Skipped)
	}

	if info, err := os.Stat(filepath.Join(root, "tasks")); err != nil || !info.IsDir() {
		t.Errorf("tasks/ не создан: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "task-template.md")); err != nil {
		t.Errorf("шаблон не создан: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".task-tracker", "schema.json")); err != nil {
		t.Errorf("schema.json не создан: %v", err)
	}
	projPath := filepath.Join(root, "projects", "myvault.md")
	raw, err := os.ReadFile(projPath)
	if err != nil {
		t.Fatalf("проект не создан: %v", err)
	}
	if !strings.Contains(string(raw), "id_prefix: MYV") || !strings.Contains(string(raw), "next_id: 1") {
		t.Errorf("проект без корректных id_prefix/next_id:\n%s", raw)
	}
}

func TestScaffoldPartialVaultFillsOnlyGaps(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	existingTemplate := "---\nсвоя разметка шаблона\n---\n"
	templatePath := filepath.Join(root, "templates", "task-template.md")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, []byte(existingTemplate), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Scaffold(root, ScaffoldOptions{Project: "demo"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if len(res.Skipped) != 2 || res.Skipped[0] != "tasks/" || res.Skipped[1] != "templates/task-template.md" {
		t.Errorf("Skipped = %v, ожидались tasks/ и templates/task-template.md", res.Skipped)
	}
	want := []string{".task-tracker/schema.json", "projects/demo.md"}
	if len(res.Created) != len(want) {
		t.Fatalf("Created = %v, ожидалось %v", res.Created, want)
	}
	for i, w := range want {
		if res.Created[i] != w {
			t.Errorf("Created[%d] = %q, ожидалось %q", i, res.Created[i], w)
		}
	}

	raw, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != existingTemplate {
		t.Errorf("существующий шаблон должен остаться нетронутым, получено:\n%s", raw)
	}
}

func TestScaffoldCompleteVaultChangesNothing(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, ScaffoldOptions{Project: "demo"}); err != nil {
		t.Fatalf("первый Scaffold: %v", err)
	}

	before := snapshotDir(t, root)
	res, err := Scaffold(root, ScaffoldOptions{Project: "demo"})
	if err != nil {
		t.Fatalf("повторный Scaffold: %v", err)
	}
	if len(res.Created) != 0 {
		t.Errorf("на готовом vault ничего не должно создаваться, Created = %v", res.Created)
	}
	want := []string{"tasks/", "templates/task-template.md", ".task-tracker/schema.json", "projects/demo.md"}
	if len(res.Skipped) != len(want) {
		t.Fatalf("Skipped = %v, ожидалось %v", res.Skipped, want)
	}
	after := snapshotDir(t, root)
	if before != after {
		t.Errorf("повторный запуск не должен менять ни одного байта на диске:\nбыло:\n%s\nстало:\n%s", before, after)
	}
}

func TestScaffoldCustomProjectAndPrefix(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, ScaffoldOptions{Project: "acme", IDPrefix: "ACM"}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "projects", "acme.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "id_prefix: ACM") {
		t.Errorf("id_prefix не подхватил явное значение:\n%s", raw)
	}
}

func TestScaffoldDefaultIDPrefixFromHyphenatedName(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, ScaffoldOptions{Project: "task-tracker"}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "projects", "task-tracker.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "id_prefix: TT") {
		t.Errorf("ожидался инициальный префикс TT:\n%s", raw)
	}
}

// TestScaffoldTemplateMatchesWhatTtReads — проверка согласованности не
// глазами: реально создаёт таску через New поверх созданного Scaffold
// шаблона и проверяет, что результат разбирается и несёт все поля,
// которые New обязан заполнить.
func TestScaffoldTemplateMatchesWhatTtReads(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, ScaffoldOptions{Project: "demo", IDPrefix: "DEM"}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	res, err := New(root, NewOptions{Project: "demo", Title: "Проверка шаблона"})
	if err != nil {
		t.Fatalf("New поверх шаблона от Scaffold: %v", err)
	}
	raw, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	task, parseErr := vault.Parse(raw)
	if parseErr != nil {
		t.Fatalf("созданная таска не разбирается: %v\n%s", parseErr, raw)
	}
	if task.ID != "DEM-001" || task.Title != "Проверка шаблона" || task.Status != "ready" ||
		task.Project != "demo" || task.Priority != "medium" || task.Created == "" {
		t.Errorf("таска неполна: %+v", task)
	}
	if strings.Contains(string(raw), "tp.date.now") || strings.Contains(string(raw), "tp.file.title") {
		t.Errorf("плейсхолдеры Templater обязаны подставиться New:\n%s", raw)
	}

	// Схема, созданная Scaffold, обязана быть тем же контрактом, что вшит в
	// бинарник — иначе доска и плагин в новом vault увидят другие лейны.
	schema, err := model.LoadSchema(SchemaPath(root))
	if err != nil {
		t.Fatalf("схема не читается: %v", err)
	}
	if canon, known := schema.Normalize("ready"); !known || canon != "ready" {
		t.Errorf("схема из Scaffold не знает статус ready")
	}
}

func snapshotDir(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			b.WriteString("DIR " + rel + "\n")
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.WriteString("FILE " + rel + " " + string(content) + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
