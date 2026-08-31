package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readTask(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lockPath(root, id string) string {
	return filepath.Join(root, ".locks", id+".lock")
}

func TestSetStatus(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer

	if err := Set(&buf, root, "ALP-1", "status", "in-progress", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := readTask(t, root, "tasks/alpha/one.md")
	if !strings.Contains(got, "status: in-progress") {
		t.Errorf("статус не записан:\n%s", got)
	}
}

func TestSetNormalizesStatusValue(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer

	if err := Set(&buf, root, "ALP-1", "status", "IN_PROGRESS", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := readTask(t, root, "tasks/alpha/one.md")
	if !strings.Contains(got, "status: in-progress") {
		t.Errorf("статус обязан записываться в каноническом виде:\n%s", got)
	}
}

func TestSetStatusDoneFillsCompleted(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer

	if err := Set(&buf, root, "ALP-1", "status", "done", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := readTask(t, root, "tasks/alpha/one.md")
	if !strings.Contains(got, "completed: 20") {
		t.Errorf("completed не проставлен:\n%s", got)
	}
}

func TestSetStatusOutOfProgressClearsClaimAndLock(t *testing.T) {
	root := fixtureVault(t)
	p := filepath.Join(root, "tasks", "beta", "four.md")
	body := "---\nid: BET-2\ntitle: четвёртая\nstatus: in-progress\nproject: beta\npriority: high\nready_at:\nclaim:\n  agent: claude\n  host: DESK\n  branch: avk\n  started: 2026-08-30\n---\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	lockDir := lockPath(root, "BET-2")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Set(&buf, root, "BET-2", "status", "ready", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := readTask(t, root, "tasks/beta/four.md")
	if strings.Contains(got, "agent: claude") {
		t.Errorf("claim не снят при уходе из работы:\n%s", got)
	}
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Error("замок не снят при уходе из работы")
	}
	if !strings.Contains(got, "ready_at: 20") {
		t.Errorf("ready_at не проставлен:\n%s", got)
	}
}

func TestSetRejectsBlockedTakeover(t *testing.T) {
	root := fixtureVault(t)
	p := filepath.Join(root, "tasks", "alpha", "one.md")
	body := "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\npriority: high\nblocked_by: [BET-2]\n---\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := Set(&buf, root, "ALP-1", "status", "in-progress", "claude")
	if err == nil {
		t.Fatal("взятие в работу заблокированной таски обязано отклоняться")
	}
	if !strings.Contains(err.Error(), "BET-2") {
		t.Errorf("ошибка обязана называть блокер, получено: %v", err)
	}
	if got := readTask(t, root, "tasks/alpha/one.md"); !strings.Contains(got, "status: ready") {
		t.Error("отклонённая правка не должна менять файл")
	}
	if _, err := os.Stat(lockPath(root, "ALP-1")); !os.IsNotExist(err) {
		t.Error("отклонённый переход не должен оставлять замок")
	}
}

func TestSetUnknownTask(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer
	if err := Set(&buf, root, "НЕТ-1", "status", "ready", "claude"); err == nil {
		t.Fatal("ожидалась ошибка про ненайденную таску")
	}
}

func TestSetRefusesUnknownField(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer
	if err := Set(&buf, root, "ALP-1", "выдумка", "значение", "claude"); err == nil {
		t.Fatal("незнакомый ключ обязан отклоняться, иначе опечатка тихо добавит мусор во фронтматтер")
	}
}

func TestSetRefusesBrokenTask(t *testing.T) {
	root := fixtureVault(t)
	p := filepath.Join(root, "tasks", "alpha", "broken.md")
	if err := os.WriteFile(p, []byte("---\nid: ALP-9\ntitle: Баг (прод): всё сломалось\nstatus: ready\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Set(&buf, root, "ALP-9", "status", "done", "claude"); err == nil {
		t.Fatal("правка непарсящейся таски обязана отклоняться: неизвестно, что там на самом деле")
	}
}

func TestSetChangesOnlyOneLine(t *testing.T) {
	root := fixtureVault(t)
	before := readTask(t, root, "tasks/alpha/one.md")
	var buf bytes.Buffer
	if err := Set(&buf, root, "ALP-1", "priority", "low", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	after := readTask(t, root, "tasks/alpha/one.md")

	b := strings.Split(before, "\n")
	a := strings.Split(after, "\n")
	if len(b) != len(a) {
		t.Fatalf("изменилось число строк: %d -> %d", len(b), len(a))
	}
	diff := 0
	for i := range b {
		if b[i] != a[i] {
			diff++
		}
	}
	if diff != 1 {
		t.Fatalf("изменилось строк: %d, ожидалась одна\n%s", diff, after)
	}
}

// Замок живёт, пока таска в работе: bash-агенты снимают его только при уходе
// из in-progress, и tt обязан вести себя так же.
func TestSetTakeoverHoldsLock(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer
	if err := Set(&buf, root, "ALP-1", "status", "in-progress", "claude"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := os.Stat(lockPath(root, "ALP-1")); err != nil {
		t.Errorf("замок не выставлен при взятии в работу: %v", err)
	}
}

// Повтор той же команды не должен упираться в замок, поставленный ею же:
// у таски в работе замок стоит штатно, и это не чужой захват.
func TestSetTakeoverIsIdempotent(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer
	if err := Set(&buf, root, "ALP-1", "status", "in-progress", "claude"); err != nil {
		t.Fatalf("первое взятие: %v", err)
	}
	if err := Set(&buf, root, "ALP-1", "status", "in-progress", "claude"); err != nil {
		t.Fatalf("повторное взятие: %v", err)
	}
	if _, err := os.Stat(lockPath(root, "ALP-1")); err != nil {
		t.Errorf("замок пропал: %v", err)
	}
}

// Чужой claim у таски в работе не отдаётся, даже когда замок на месте.
func TestSetRefusesForeignClaimInProgress(t *testing.T) {
	root := fixtureVault(t)
	p := filepath.Join(root, "tasks", "beta", "four.md")
	body := "---\nid: BET-2\ntitle: четвёртая\nstatus: in-progress\nproject: beta\npriority: high\nclaim:\n  agent: codex\n  host: DESK\n  branch: avk\n  started: 2026-08-30\n---\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lockPath(root, "BET-2"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := Set(&buf, root, "BET-2", "status", "in-progress", "claude")
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("чужой claim обязан отклоняться с именем владельца, получено: %v", err)
	}
}

func TestSetRefusesForeignLock(t *testing.T) {
	root := fixtureVault(t)
	lockDir := lockPath(root, "ALP-1")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Set(&buf, root, "ALP-1", "status", "in-progress", "claude"); err == nil {
		t.Fatal("взятие в работу таски с чужим замком обязано отклоняться")
	}
	if _, err := os.Stat(lockDir); err != nil {
		t.Errorf("чужой замок сорван: %v", err)
	}
	if got := readTask(t, root, "tasks/alpha/one.md"); !strings.Contains(got, "status: ready") {
		t.Errorf("отклонённая правка не должна менять файл:\n%s", got)
	}
}
