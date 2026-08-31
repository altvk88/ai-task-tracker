package taskop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alkulagin-creator/tt/internal/vault"
)

// newFixtureVault — минимальный vault с одним проектом и штатным шаблоном
// таски (тем же, что лежит в templates/task-template.md живого vault).
func newFixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"projects/alpha.md": "---\nproject: alpha\nid_prefix: ALP\nnext_id: 1\n---\n\n# alpha\n",
		"templates/task-template.md": "---\n" +
			"id:\n" +
			"title: \"<% tp.file.title %>\"\n" +
			"status: backlog\n" +
			"project:\n" +
			"priority: medium\n" +
			"due:\n" +
			"tags: []\n" +
			"created: <% tp.date.now(\"YYYY-MM-DD\") %>\n" +
			"completed:\n" +
			"blocked_by:\n" +
			"effort:\n" +
			"actual:\n" +
			"ready_at:\n" +
			"attempts: 0\n" +
			"spec:\n" +
			"verify:\n" +
			"result:\n" +
			"claim:\n" +
			"---\n\n" +
			"## Description\n\n\n\n## Acceptance Criteria\n\n- [ ]\n\n## Notes\n\n\n\n## Open Questions\n\n\n\n## Log\n\n- <% tp.date.now(\"YYYY-MM-DD\") %>: Created\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "tasks", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func readProjectFile(t *testing.T, root, project string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "projects", project+".md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewCreatesReadableTask(t *testing.T) {
	root := newFixtureVault(t)
	res, err := New(root, NewOptions{Project: "alpha", Title: "Простая задача"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if res.ID != "ALP-001" {
		t.Errorf("ID = %q, ожидался ALP-001", res.ID)
	}
	raw, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	task, parseErr := vault.Parse(raw)
	if parseErr != nil {
		t.Fatalf("созданная таска не разбирается: %v", parseErr)
	}
	if task.ID != "ALP-001" {
		t.Errorf("Task.ID = %q", task.ID)
	}
	if task.Title != "Простая задача" {
		t.Errorf("Task.Title = %q", task.Title)
	}
	if task.Project != "alpha" {
		t.Errorf("Task.Project = %q", task.Project)
	}
}

func TestNewIncrementsNextIDExactlyOnce(t *testing.T) {
	root := newFixtureVault(t)
	if _, err := New(root, NewOptions{Project: "alpha", Title: "Первая"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if !strings.Contains(readProjectFile(t, root, "alpha"), "next_id: 2") {
		t.Errorf("next_id обязан стать 2:\n%s", readProjectFile(t, root, "alpha"))
	}
}

func TestNewQuotesTitleWithColon(t *testing.T) {
	root := newFixtureVault(t)
	res, err := New(root, NewOptions{Project: "alpha", Title: "Баг (прод): всё сломалось"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw, _ := os.ReadFile(res.Path)
	if !strings.Contains(string(raw), `title: "Баг (прод): всё сломалось"`) {
		t.Errorf("заголовок с \": \" обязан быть закавычен:\n%s", raw)
	}
	if _, err := vault.Parse(raw); err != nil {
		t.Fatalf("файл не разбирается: %v", err)
	}
}

func TestNewWithoutDependsOnIsReady(t *testing.T) {
	root := newFixtureVault(t)
	res, err := New(root, NewOptions{Project: "alpha", Title: "Без зависимостей"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if res.Status != "ready" {
		t.Errorf("Status = %q, ожидался ready", res.Status)
	}
	raw, _ := os.ReadFile(res.Path)
	task, _ := vault.Parse(raw)
	if task.Status != "ready" {
		t.Errorf("Task.Status = %q", task.Status)
	}
	if task.ReadyAt == "" {
		t.Error("ready_at обязан быть проставлен")
	}
}

func TestNewWithDependsOnIsBacklog(t *testing.T) {
	root := newFixtureVault(t)
	res, err := New(root, NewOptions{Project: "alpha", Title: "С зависимостью", DependsOn: []string{"ALP-000"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if res.Status != "backlog" {
		t.Errorf("Status = %q, ожидался backlog", res.Status)
	}
	raw, _ := os.ReadFile(res.Path)
	task, _ := vault.Parse(raw)
	if task.Status != "backlog" {
		t.Errorf("Task.Status = %q", task.Status)
	}
	if task.ReadyAt != "" {
		t.Errorf("ready_at обязан быть пустым, получено %q", task.ReadyAt)
	}
	if len(task.BlockedBy) != 1 || task.BlockedBy[0] != "ALP-000" {
		t.Errorf("BlockedBy = %v", task.BlockedBy)
	}
}

func TestNewUnknownDependencyWarnsNotRejects(t *testing.T) {
	root := newFixtureVault(t)
	res, err := New(root, NewOptions{Project: "alpha", Title: "Зависит от будущей", DependsOn: []string{"ALP-999"}})
	if err != nil {
		t.Fatalf("New обязан создать таску, а не отказывать из-за несуществующей зависимости: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "ALP-999") {
		t.Errorf("Warnings = %v, ожидалось предупреждение про ALP-999", res.Warnings)
	}
}

func TestNewUnknownProject(t *testing.T) {
	root := newFixtureVault(t)
	if _, err := New(root, NewOptions{Project: "нет-такого", Title: "Что-то"}); err == nil {
		t.Fatal("ожидалась ошибка про несуществующий проект")
	} else if kind, ok := KindOf(err); !ok || kind != KindNotFound {
		t.Errorf("ожидался KindNotFound, получено %v (ok=%v)", err, ok)
	}
}

func TestNewMissingTemplate(t *testing.T) {
	root := newFixtureVault(t)
	if err := os.Remove(filepath.Join(root, "templates", "task-template.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, NewOptions{Project: "alpha", Title: "Что-то"}); err == nil {
		t.Fatal("ожидалась ошибка про отсутствующий шаблон")
	}
}

func TestNewFillsLogPlaceholder(t *testing.T) {
	root := newFixtureVault(t)
	res, err := New(root, NewOptions{Project: "alpha", Title: "Дата в логе"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw, _ := os.ReadFile(res.Path)
	if strings.Contains(string(raw), "tp.date.now") {
		t.Errorf("плейсхолдер Templater в теле файла не заменён:\n%s", raw)
	}
	if !strings.Contains(string(raw), ": Created") {
		t.Errorf("строка Log потеряна:\n%s", raw)
	}
}

func TestNewFilenameCollisionGetsSuffix(t *testing.T) {
	root := newFixtureVault(t)
	first, err := New(root, NewOptions{Project: "alpha", Title: "Одинаковый заголовок"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	second, err := New(root, NewOptions{Project: "alpha", Title: "Одинаковый заголовок"})
	if err != nil {
		t.Fatalf("New (повтор заголовка): %v", err)
	}
	if first.Path == second.Path {
		t.Fatalf("второй файл обязан получить другое имя, получено то же: %s", second.Path)
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Errorf("первый файл потерян: %v", err)
	}
	if _, err := os.Stat(second.Path); err != nil {
		t.Errorf("второй файл не создан: %v", err)
	}
}

func TestNewConcurrentCallsGetDistinctIDs(t *testing.T) {
	root := newFixtureVault(t)
	const n = 10
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := New(root, NewOptions{Project: "alpha", Title: "Параллельная задача"})
			ids[i] = res.ID
			errs[i] = err
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("New #%d: %v", i, err)
		}
		if seen[ids[i]] {
			t.Fatalf("ID %s выдан дважды", ids[i])
		}
		seen[ids[i]] = true
	}
	if !strings.Contains(readProjectFile(t, root, "alpha"), "next_id: 11") {
		t.Errorf("next_id обязан вырасти ровно на %d:\n%s", n, readProjectFile(t, root, "alpha"))
	}
}
