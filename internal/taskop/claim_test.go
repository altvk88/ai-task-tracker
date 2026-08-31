package taskop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/vault"
)

// claimFixture — vault с проектом alpha и парой тасок: свободной ALP-1 и
// заблокированной ALP-2. repo проекта по умолчанию не задан, поэтому ветку
// взять неоткуда — ровно тот случай, когда branch обязан остаться пустым.
func claimFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"projects/alpha.md": "---\nproject: alpha\nid_prefix: ALP\nnext_id: 3\nrepo:\n---\n",
		"tasks/alpha/one.md": "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-08-01\nattempts: 0\nclaim:\n---\n\n" +
			"## Description\n\nТело таски.\n",
		"tasks/alpha/two.md": "---\nid: ALP-2\ntitle: вторая\nstatus: ready\nproject: alpha\n" +
			"priority: low\ncreated: 2026-08-01\nblocked_by: [ALP-1]\nattempts: 0\nclaim:\n---\n",
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

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lockDir(root, id string) string { return filepath.Join(root, ".locks", id+".lock") }

func locked(root, id string) bool {
	_, err := os.Stat(lockDir(root, id))
	return err == nil
}

// stripVolatile убирает строки, которые команда меняет по праву: статус и
// блок claim. Остаток обязан совпасть байт в байт.
func stripVolatile(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "claim:") {
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") {
				i++
			}
			continue
		}
		if strings.HasPrefix(lines[i], "status:") {
			continue
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}

func TestClaimWritesBlockAndLock(t *testing.T) {
	root := claimFixture(t)
	res, err := Claim(root, "ALP-1", "claude")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if res.From != "ready" || res.To != "in-progress" {
		t.Errorf("Result = %q -> %q", res.From, res.To)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if !strings.Contains(got, "status: in-progress") {
		t.Errorf("статус не записан:\n%s", got)
	}
	for _, want := range []string{"claim:\n  agent: claude\n", "  host: ", "  started: 20"} {
		if !strings.Contains(got, want) {
			t.Errorf("в блоке claim нет %q:\n%s", want, got)
		}
	}
	if !locked(root, "ALP-1") {
		t.Error("замок не поставлен")
	}
	if res.Task.Claim == nil || res.Task.Claim.Agent != "claude" {
		t.Errorf("перечитанная таска без claim: %+v", res.Task.Claim)
	}
}

func TestClaimLeavesBranchEmptyWithoutRepo(t *testing.T) {
	root := claimFixture(t)
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if strings.Contains(got, "branch:") {
		t.Errorf("недоступный репозиторий обязан оставлять branch пустым:\n%s", got)
	}
}

func TestClaimTakesBranchFromProjectRepo(t *testing.T) {
	root := claimFixture(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-b", "task/ALP-1").CombinedOutput(); err != nil {
		t.Skipf("git недоступен: %v (%s)", err, out)
	}
	projPath := filepath.Join(root, "projects", "alpha.md")
	if err := vault.SetField(projPath, "repo", filepath.ToSlash(repo)); err != nil {
		t.Fatal(err)
	}
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if !strings.Contains(got, "  branch: task/ALP-1\n") {
		t.Errorf("ветка не взята из репозитория проекта:\n%s", got)
	}
}

func TestClaimTouchesOnlyStatusAndClaim(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	before := read(t, path)
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if after := read(t, path); stripVolatile(before) != stripVolatile(after) {
		t.Fatalf("тронуты строки помимо статуса и claim:\nбыло:\n%s\nстало:\n%s", before, after)
	}
}

func TestClaimPreservesCRLF(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	crlf := strings.ReplaceAll(read(t, path), "\n", "\r\n")
	if err := os.WriteFile(path, []byte(crlf), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := read(t, path)
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Errorf("файл стал смешанным по переводам строк:\n%q", got)
	}
}

func TestClaimRejectsForeignClaim(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	before := read(t, path)

	_, err := Claim(root, "ALP-1", "другой")
	if err == nil {
		t.Fatal("чужой claim обязан отклоняться")
	}
	if kind, ok := KindOf(err); !ok || kind != KindRejected {
		t.Errorf("вид отказа = %v (ok=%v), ожидался KindRejected", kind, ok)
	}
	if after := read(t, path); after != before {
		t.Errorf("файл изменён при отказе:\n%s", after)
	}
}

func TestClaimIsIdempotentForSameAgent(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("первый Claim: %v", err)
	}
	first := read(t, path)
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("повторный Claim своим агентом: %v", err)
	}
	second := read(t, path)
	if first != second {
		t.Errorf("повторный claim изменил файл:\nбыло:\n%s\nстало:\n%s", first, second)
	}
	if n := strings.Count(second, "claim:"); n != 1 {
		t.Errorf("блок claim продублирован (%d раз):\n%s", n, second)
	}
	if !locked(root, "ALP-1") {
		t.Error("замок пропал после повторного claim")
	}
}

func TestClaimRejectsBlockedTask(t *testing.T) {
	root := claimFixture(t)
	_, err := Claim(root, "ALP-2", "claude")
	if err == nil {
		t.Fatal("заблокированная таска обязана отклоняться")
	}
	if !strings.Contains(err.Error(), "ALP-1") {
		t.Errorf("в отказе нет ID блокера: %v", err)
	}
	if locked(root, "ALP-2") {
		t.Error("на отклонённой таске остался замок")
	}
}

func TestReleaseClearsClaimAndLock(t *testing.T) {
	root := claimFixture(t)
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatal(err)
	}
	res, err := Release(root, "ALP-1")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if res.To != "ready" {
		t.Errorf("статус после release = %q", res.To)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if strings.Contains(got, "agent: claude") {
		t.Errorf("claim не снят:\n%s", got)
	}
	if !strings.Contains(got, "status: ready") {
		t.Errorf("статус не ready:\n%s", got)
	}
	if locked(root, "ALP-1") {
		t.Error("замок не снят")
	}
}

func TestResetForcesForeignClaimAndCountsAttempt(t *testing.T) {
	root := claimFixture(t)
	if _, err := Claim(root, "ALP-1", "чужой"); err != nil {
		t.Fatal(err)
	}
	res, err := Reset(root, "ALP-1")
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if res.To != "ready" {
		t.Errorf("статус после reset = %q", res.To)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if strings.Contains(got, "agent: чужой") {
		t.Errorf("чужой claim не снят:\n%s", got)
	}
	if !strings.Contains(got, "attempts: 1") {
		t.Errorf("attempts не увеличен:\n%s", got)
	}
	if locked(root, "ALP-1") {
		t.Error("замок не снят")
	}
	if res.Task.Attempts != 1 {
		t.Errorf("Attempts в результате = %d", res.Task.Attempts)
	}
}

func TestResetWorksOnStuckTaskWithoutClaimBlock(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	if err := vault.SetField(path, "status", "in-progress"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lockDir(root, "ALP-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Reset(root, "ALP-1"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if locked(root, "ALP-1") {
		t.Error("замок упавшей сессии не снят")
	}
	if got := read(t, path); !strings.Contains(got, "status: ready") {
		t.Errorf("статус не сброшен:\n%s", got)
	}
}

func TestClaimUnknownTask(t *testing.T) {
	root := claimFixture(t)
	_, err := Claim(root, "ALP-99", "claude")
	if kind, ok := KindOf(err); !ok || kind != KindNotFound {
		t.Errorf("вид отказа = %v (ok=%v), ожидался KindNotFound", kind, ok)
	}
}
