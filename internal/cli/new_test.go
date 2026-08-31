package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// newFixtureVault — vault с одним проектом и штатным шаблоном таски, для
// проверки, что cli.New — действительно тонкая обёртка над taskop.New.
func newFixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"projects/alpha.md":          "---\nproject: alpha\nid_prefix: ALP\nnext_id: 1\n---\n",
		"templates/task-template.md": "---\nid:\ntitle: \"<% tp.file.title %>\"\nstatus: backlog\nproject:\npriority: medium\ncreated:\nblocked_by:\nready_at:\nspec:\nclaim:\n---\n",
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

func TestCLINewCreatesTaskAndPrintsID(t *testing.T) {
	root := newFixtureVault(t)
	var buf bytes.Buffer
	if err := New(&buf, root, taskop.NewOptions{Project: "alpha", Title: "Первая задача"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if !strings.Contains(buf.String(), "ALP-001") {
		t.Errorf("вывод обязан содержать выданный ID:\n%s", buf.String())
	}
}

func TestCLINewPrintsDependencyWarning(t *testing.T) {
	root := newFixtureVault(t)
	var buf bytes.Buffer
	if err := New(&buf, root, taskop.NewOptions{Project: "alpha", Title: "С зависимостью", DependsOn: []string{"ALP-999"}}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if !strings.Contains(buf.String(), "ALP-999") {
		t.Errorf("вывод обязан предупреждать про несуществующую зависимость:\n%s", buf.String())
	}
}

func TestCLINewUnknownProjectFails(t *testing.T) {
	root := newFixtureVault(t)
	var buf bytes.Buffer
	if err := New(&buf, root, taskop.NewOptions{Project: "нет-такого", Title: "Что-то"}); err == nil {
		t.Fatal("ожидалась ошибка про несуществующий проект")
	}
}
