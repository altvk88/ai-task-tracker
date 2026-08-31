package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"tasks/alpha/one.md":  "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\npriority: high\n---\n",
		"tasks/alpha/two.md":  "---\nid: ALP-2\ntitle: вторая\nstatus: done\nproject: alpha\npriority: low\n---\n",
		"tasks/beta/three.md": "---\nid: BET-1\ntitle: третья\nstatus: ready\nproject: beta\npriority: medium\n---\n",
		"tasks/beta/four.md":  "---\nid: BET-2\ntitle: четвёртая\nstatus: in_progress\nproject: beta\npriority: high\n---\n",
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
	return root
}

func TestListFilters(t *testing.T) {
	root := fixtureVault(t)

	t.Run("без фильтров — все таски", func(t *testing.T) {
		var buf bytes.Buffer
		if err := List(&buf, root, ListOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"ALP-1", "ALP-2", "BET-1", "BET-2"} {
			if !strings.Contains(buf.String(), id) {
				t.Errorf("в выводе нет %s:\n%s", id, buf.String())
			}
		}
	})

	t.Run("фильтр по проекту", func(t *testing.T) {
		var buf bytes.Buffer
		if err := List(&buf, root, ListOptions{Project: "beta"}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "ALP-") {
			t.Errorf("просочился чужой проект:\n%s", buf.String())
		}
	})

	t.Run("фильтр по статусу понимает историческое написание", func(t *testing.T) {
		var buf bytes.Buffer
		if err := List(&buf, root, ListOptions{Status: "in-progress"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "BET-2") {
			t.Errorf("таска со status: in_progress не найдена по in-progress:\n%s", buf.String())
		}
		if strings.Contains(buf.String(), "ALP-1") {
			t.Errorf("лишняя таска в выводе:\n%s", buf.String())
		}
	})

	t.Run("неизвестный статус в фильтре — ошибка", func(t *testing.T) {
		var buf bytes.Buffer
		if err := List(&buf, root, ListOptions{Status: "выдумка"}); err == nil {
			t.Fatal("ожидалась ошибка про неизвестный статус")
		}
	})

	t.Run("JSON пригоден для агентов", func(t *testing.T) {
		var buf bytes.Buffer
		if err := List(&buf, root, ListOptions{Status: "ready", JSON: true}); err != nil {
			t.Fatal(err)
		}
		var got []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("невалидный JSON: %v\n%s", err, buf.String())
		}
		if len(got) != 2 {
			t.Fatalf("тасок в JSON: %d, ожидалось 2", len(got))
		}
		for _, g := range got {
			if g.Status != "ready" {
				t.Errorf("в выводе статус %q", g.Status)
			}
		}
	})

	t.Run("битая таска видна с пометкой", func(t *testing.T) {
		root := fixtureVault(t)
		p := filepath.Join(root, "tasks", "alpha", "broken.md")
		if err := os.WriteFile(p, []byte("---\ntitle: Баг (прод): всё сломалось\nstatus: ready\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := List(&buf, root, ListOptions{}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "BROKEN") {
			t.Errorf("битая таска не помечена:\n%s", buf.String())
		}
	})
}

// Таблица обязана держать контракт «одна таска — одна строка»: многострочная
// ошибка yaml.v3 иначе разваливает вывод, и его не разобрать ни глазами,
// ни скриптом. А у битой таски пустой id, поэтому строка без имени файла
// не позволяет понять, что именно чинить.
func TestListTableKeepsOneLinePerTask(t *testing.T) {
	root := fixtureVault(t)
	broken := filepath.Join(root, "tasks", "alpha", "multiline-error.md")
	// Дубль ключа даёт многострочную ошибку: "yaml: unmarshal errors:\n  line N: ..."
	body := "---\nid: ALP-9\ntitle: битая\nstatus: ready\nproject: alpha\npriority: low\ncompleted:\ncompleted:\n---\n"
	if err := os.WriteFile(broken, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	noID := filepath.Join(root, "tasks", "alpha", "no-id.md")
	if err := os.WriteFile(noID, []byte("---\ntitle: Баг (прод): всё сломалось\nstatus: ready\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := List(&buf, root, ListOptions{}); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimRight(buf.String(), "\n")
	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("строк в выводе %d, ожидалось 6 (4 целых + 2 битых):\n%s", len(lines), out)
	}
	for i, l := range lines {
		if strings.HasPrefix(l, " ") {
			t.Errorf("строка %d начинается с отступа — это обрывок многострочной ошибки: %q", i+1, l)
		}
	}
	if !strings.Contains(out, "no-id.md") {
		t.Errorf("у таски без id не показано имя файла, её нечем опознать:\n%s", out)
	}
}
