package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/vault"
)

// fixVault прогоняет doctor --fix по битому vault и отдаёт корень и вывод.
func fixVault(t *testing.T) (string, string) {
	t.Helper()
	root := brokenVault(t)
	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, true); err != nil {
		t.Fatalf("Doctor --fix: %v", err)
	}
	return root, buf.String()
}

func TestDoctorFixNormalizesStatus(t *testing.T) {
	root, out := fixVault(t)
	body := readTask(t, root, "tasks/alpha/legacy.md")
	if !strings.Contains(body, "status: in-progress") {
		t.Errorf("статус ALP-10 не нормализован:\n%s\nвывод:\n%s", body, out)
	}
	if strings.Contains(body, "in_progress") {
		t.Errorf("историческое написание осталось в файле:\n%s", body)
	}
}

func TestDoctorFixClearsStaleClaim(t *testing.T) {
	root, out := fixVault(t)
	body := readTask(t, root, "tasks/alpha/stale.md")
	if strings.Contains(body, "agent: claude") {
		t.Errorf("залипший claim ALP-11 не снят:\n%s\nвывод:\n%s", body, out)
	}
}

func TestDoctorFixQuotesBrokenTitle(t *testing.T) {
	root, out := fixVault(t)
	body := readTask(t, root, "tasks/alpha/broken.md")
	if !strings.Contains(body, `title: "Баг (прод): всё сломалось"`) {
		t.Errorf("заголовок ALP-9 не закавычен:\n%s\nвывод:\n%s", body, out)
	}
	task, err := vault.Parse([]byte(body))
	if err != nil {
		t.Fatalf("после починки файл всё ещё не разбирается: %v", err)
	}
	if task.Title != "Баг (прод): всё сломалось" {
		t.Errorf("заголовок после починки = %q", task.Title)
	}
}

func TestDoctorFixRemovesOrphanLock(t *testing.T) {
	root, out := fixVault(t)
	if _, err := os.Stat(filepath.Join(root, ".locks", "ALP-777.lock")); !os.IsNotExist(err) {
		t.Errorf("осиротевший замок ALP-777 не снят (%v)\nвывод:\n%s", err, out)
	}
}

func TestDoctorFixLeavesNothingFixable(t *testing.T) {
	root, _ := fixVault(t)

	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, false); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "*") {
			t.Errorf("после починки осталась починяемая проблема: %q\nвесь вывод:\n%s", line, buf.String())
		}
	}
}

func TestDoctorFixKeepsUnfixableInReport(t *testing.T) {
	_, out := fixVault(t)
	for _, want := range []string{"ALP-999", "priority", "дубл"} {
		if !strings.Contains(out, want) {
			t.Errorf("непочиняемая проблема %q пропала из отчёта:\n%s", want, out)
		}
	}
}

// Живой случай (8 таск LAL): помимо незакавыченного заголовка у файла result
// записан мапой, и после закавычивания он всё равно не разбирается. Такой файл
// нельзя ни помечать починяемым, ни трогать.
func TestDoctorFixSkipsQuotingThatDoesNotHelp(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "tasks", "alpha", "hopeless.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: ALP-20\ntitle: Баг (прод): всё сломалось\nstatus: ready\nproject: alpha\npriority: low\ncreated: 2026-08-01\nresult:\n  ok: да\n---\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var report bytes.Buffer
	if _, err := Doctor(&report, root, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(report.String(), "*") {
		t.Errorf("проблема помечена починяемой, хотя закавычивание её не решает:\n%s", report.String())
	}

	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, true); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != body {
		t.Errorf("--fix изменил файл, который починить не смог:\n%s", after)
	}
}

func TestDoctorFixDoesNotTouchCleanTask(t *testing.T) {
	root := brokenVault(t)
	rel := "tasks/beta/histverify.md"
	before := readTask(t, root, rel)

	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, true); err != nil {
		t.Fatal(err)
	}
	if after := readTask(t, root, rel); after != before {
		t.Errorf("--fix изменил чистую таску:\nбыло:\n%q\nстало:\n%q", before, after)
	}
}

func TestDoctorFixВосстанавливаетФенс(t *testing.T) {
	root, out := fixVault(t)

	body := readTask(t, root, "tasks/alpha/nofence.md")
	if _, err := vault.Parse([]byte(body)); err != nil {
		t.Errorf("ALP-14 после починки не разбирается: %v\nвывод:\n%s", err, out)
	}
	if !strings.Contains(body, "Тело.") {
		t.Errorf("тело ALP-14 потеряно:\n%s", body)
	}

	if got := readTask(t, root, "tasks/alpha/nofencelog.md"); got != nofenceLogTask {
		t.Errorf("ALP-15 чинить нельзя, но файл изменён:\n%q", got)
	}
}
