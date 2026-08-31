package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// tempVault раскладывает минимальный vault и отдаёт его путь.
func tempVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
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

func TestScan(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md":     "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n---\n\nтело\n",
		"tasks/alpha/two.md":     "---\nid: ALP-2\ntitle: вторая\nstatus: done\nproject: alpha\n---\n",
		"tasks/beta/three.md":    "---\nid: BET-1\ntitle: третья\nstatus: backlog\nproject: beta\n---\n",
		"tasks/_example/skip.md": "---\nid: EX-1\ntitle: образец\nstatus: ready\nproject: _example\n---\n",
		"tasks/alpha/broken.md":  "---\ntitle: Баг (прод): всё сломалось\nstatus: ready\n---\n",
		"tasks/alpha/notes.txt":  "не markdown, игнорируем",
		"dashboard.md":           "---\nid: NOPE\n---\n",
	})

	got, err := Scan(root)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("найдено %d тасок, ожидалось 4: %+v", len(got), got)
	}

	byID := map[string]bool{}
	broken := 0
	for _, task := range got {
		if task.ParseErr != "" {
			broken++
			if task.Path == "" {
				t.Error("у битой таски обязан быть путь, иначе её нельзя починить")
			}
			continue
		}
		byID[task.ID] = true
	}
	for _, want := range []string{"ALP-1", "ALP-2", "BET-1"} {
		if !byID[want] {
			t.Errorf("не найдена таска %s", want)
		}
	}
	if broken != 1 {
		t.Errorf("битых тасок %d, ожидалась 1 (незакавыченный заголовок с двоеточием)", broken)
	}
}

func TestScanNoTasksDir(t *testing.T) {
	if _, err := Scan(t.TempDir()); err == nil {
		t.Fatal("vault без каталога tasks обязан давать внятную ошибку")
	}
}

func TestByID(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md":    "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n---\n",
		"tasks/alpha/broken.md": "---\ntitle: Баг (прод): всё сломалось\nstatus: ready\n---\n",
	})
	tasks, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	idx := ByID(tasks)
	if _, ok := idx["ALP-1"]; !ok {
		t.Error("ALP-1 не попала в индекс")
	}
	if len(idx) != 1 {
		t.Errorf("в индексе %d записей, ожидалась 1: битые без ID туда не попадают", len(idx))
	}
}
