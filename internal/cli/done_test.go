package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doneFixtureVault — ALP-1 свободна, ALP-2 зависит только от неё.
func doneFixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"tasks/alpha/one.md": "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n" +
			"priority: high\nattempts: 0\nclaim:\n---\n",
		"tasks/alpha/two.md": "---\nid: ALP-2\ntitle: вторая\nstatus: backlog\nproject: alpha\n" +
			"priority: medium\nblocked_by: [ALP-1]\nattempts: 0\nclaim:\n---\n",
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

func TestDonePrintsStatusAndPromoted(t *testing.T) {
	root := doneFixtureVault(t)
	var buf bytes.Buffer
	if err := Done(&buf, root, "ALP-1", "готово", "claude"); err != nil {
		t.Fatalf("Done: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ALP-1: ready -> done") {
		t.Errorf("нет строки статуса: %q", out)
	}
	if !strings.Contains(out, "промоут в ready: ALP-2") {
		t.Errorf("нет строки промоута: %q", out)
	}
	got := readTask(t, root, "tasks/alpha/one.md")
	if !strings.Contains(got, `result: "готово"`) && !strings.Contains(got, "result: готово") {
		t.Errorf("result не записан:\n%s", got)
	}
}

func TestDoneWithoutPromotionPrintsNoPromotedLine(t *testing.T) {
	root := doneFixtureVault(t)
	var buf bytes.Buffer
	// ALP-2 сама блокеров не имеет свободных зависимых — промоут-строки быть не должно.
	if err := Done(&buf, root, "ALP-2", "", "claude"); err == nil {
		// ALP-2 в backlog, done из backlog разрешён (полной матрицы переходов нет).
		out := buf.String()
		if strings.Contains(out, "промоут в ready:") {
			t.Errorf("промоут-строка не должна печататься без промоутнутых: %q", out)
		}
	}
}
