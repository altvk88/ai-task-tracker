package taskop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doneFixture — vault с проектом alpha и связкой тасок под сценарии авто-
// промоута: ALP-1 и ALP-4 свободны, остальные зависят от них по-разному.
func doneFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"projects/alpha.md": "---\nproject: alpha\nid_prefix: ALP\nnext_id: 8\nrepo:\n---\n",
		// ALP-1 — свободная таска, её и закрываем в большинстве тестов.
		"tasks/alpha/one.md": "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-08-01\nattempts: 0\nclaim:\n---\n\n" +
			"## Description\n\nТело таски.\n",
		// ALP-2 — единственный блокер ALP-1, обязана промоутнуться.
		"tasks/alpha/two.md": "---\nid: ALP-2\ntitle: вторая\nstatus: backlog\nproject: alpha\n" +
			"priority: medium\ncreated: 2026-08-01\nblocked_by: [ALP-1]\nattempts: 0\nclaim:\n---\n",
		// ALP-3 — два блокера, ALP-1 и ALP-4: промоут только когда оба done.
		"tasks/alpha/three.md": "---\nid: ALP-3\ntitle: третья\nstatus: backlog\nproject: alpha\n" +
			"priority: medium\ncreated: 2026-08-01\nblocked_by: [ALP-1, ALP-4]\nattempts: 0\nclaim:\n---\n",
		// ALP-4 — вторая свободная таска, второй блокер ALP-3.
		"tasks/alpha/four.md": "---\nid: ALP-4\ntitle: четвёртая\nstatus: ready\nproject: alpha\n" +
			"priority: medium\ncreated: 2026-08-01\nready_at: 2026-08-01\nattempts: 0\nclaim:\n---\n",
		// ALP-5 — заблокирована ALP-1, но статус blocked: промоут не трогает.
		"tasks/alpha/five.md": "---\nid: ALP-5\ntitle: пятая\nstatus: blocked\nproject: alpha\n" +
			"priority: low\ncreated: 2026-08-01\nblocked_by: [ALP-1]\nattempts: 0\nclaim:\n---\n",
		// ALP-6 — заблокирована ALP-1, но статус hold: промоут не трогает.
		"tasks/alpha/six.md": "---\nid: ALP-6\ntitle: шестая\nstatus: hold\nproject: alpha\n" +
			"priority: low\ncreated: 2026-08-01\nblocked_by: [ALP-1]\nattempts: 0\nclaim:\n---\n",
		// ALP-7 — backlog вовсе без блокеров: посторонний кандидат, закрытие
		// ALP-1 не должно его задевать (проверка области промоута).
		"tasks/alpha/seven.md": "---\nid: ALP-7\ntitle: седьмая\nstatus: backlog\nproject: alpha\n" +
			"priority: low\ncreated: 2026-08-01\nattempts: 0\nclaim:\n---\n",
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

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestDonePromotesSoleDependent(t *testing.T) {
	root := doneFixture(t)
	res, err := Done(root, "ALP-1", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if res.Result.From != "ready" || res.Result.To != "done" {
		t.Errorf("Result = %q -> %q", res.Result.From, res.Result.To)
	}
	if !containsID(res.Promoted, "ALP-2") {
		t.Errorf("ALP-2 не в списке промоута: %v", res.Promoted)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "two.md"))
	if !strings.Contains(got, "status: ready") {
		t.Errorf("ALP-2 не промоутнута:\n%s", got)
	}
	if !strings.Contains(got, "ready_at:") {
		t.Errorf("у ALP-2 нет ready_at:\n%s", got)
	}
}

func TestDoneDoesNotPromoteUnrelatedBacklogCandidate(t *testing.T) {
	root := doneFixture(t)
	res, err := Done(root, "ALP-1", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if containsID(res.Promoted, "ALP-7") {
		t.Errorf("посторонний кандидат ALP-7 не должен промоутиться закрытием ALP-1: %v", res.Promoted)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "seven.md"))
	if !strings.Contains(got, "status: backlog") {
		t.Errorf("ALP-7 не должна была измениться:\n%s", got)
	}
}

func TestDonePromotesOnlyAfterAllBlockersClosed(t *testing.T) {
	root := doneFixture(t)
	res, err := Done(root, "ALP-1", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if containsID(res.Promoted, "ALP-3") {
		t.Errorf("ALP-3 не должна промоутиться при одном закрытом блокере: %v", res.Promoted)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "three.md"))
	if !strings.Contains(got, "status: backlog") {
		t.Errorf("ALP-3 промоутнута раньше времени:\n%s", got)
	}

	res2, err := Done(root, "ALP-4", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if !containsID(res2.Promoted, "ALP-3") {
		t.Errorf("ALP-3 обязана промоутиться после второго блокера: %v", res2.Promoted)
	}
	got = read(t, filepath.Join(root, "tasks", "alpha", "three.md"))
	if !strings.Contains(got, "status: ready") {
		t.Errorf("ALP-3 не промоутнута после закрытия обоих блокеров:\n%s", got)
	}
}

func TestDoneDoesNotPromoteBlockedOrHold(t *testing.T) {
	root := doneFixture(t)
	res, err := Done(root, "ALP-1", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if containsID(res.Promoted, "ALP-5") || containsID(res.Promoted, "ALP-6") {
		t.Errorf("blocked/hold не должны промоутиться: %v", res.Promoted)
	}
	five := read(t, filepath.Join(root, "tasks", "alpha", "five.md"))
	if !strings.Contains(five, "status: blocked") {
		t.Errorf("ALP-5 изменена:\n%s", five)
	}
	six := read(t, filepath.Join(root, "tasks", "alpha", "six.md"))
	if !strings.Contains(six, "status: hold") {
		t.Errorf("ALP-6 изменена:\n%s", six)
	}
}

func TestDoneIsIdempotent(t *testing.T) {
	root := doneFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	if _, err := Done(root, "ALP-1", "первый результат", "claude"); err != nil {
		t.Fatalf("первый Done: %v", err)
	}
	first := read(t, path)

	res, err := Done(root, "ALP-1", "второй результат", "claude")
	if err != nil {
		t.Fatalf("повторный Done: %v", err)
	}
	second := read(t, path)
	if !strings.Contains(first, "completed:") {
		t.Fatalf("completed не проставлен после первого закрытия:\n%s", first)
	}
	// completed не должен переехать на новую дату при повторном закрытии.
	firstCompletedLine := completedLine(first)
	secondCompletedLine := completedLine(second)
	if firstCompletedLine != secondCompletedLine {
		t.Errorf("completed изменился при повторном Done: %q -> %q", firstCompletedLine, secondCompletedLine)
	}
	if len(res.Promoted) != 0 {
		t.Errorf("повторное закрытие не должно промоутить снова: %v", res.Promoted)
	}
	// А повторный ALP-2 не должен промоутнуться ещё раз (уже ready).
	two := read(t, filepath.Join(root, "tasks", "alpha", "two.md"))
	if strings.Count(two, "status:") != 1 {
		t.Errorf("статус ALP-2 задублирован:\n%s", two)
	}
}

func completedLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "completed:") {
			return line
		}
	}
	return ""
}

func TestDoneWritesResultAndQuotesColon(t *testing.T) {
	root := doneFixture(t)
	res, err := Done(root, "ALP-1", "сделано: всё ок", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if res.Result.Task.Result != "сделано: всё ок" {
		t.Errorf("result в перечитанной таске = %q", res.Result.Task.Result)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if !strings.Contains(got, `result: "сделано: всё ок"`) {
		t.Errorf("result не закавычен:\n%s", got)
	}
}

func TestDoneClearsClaimAndLock(t *testing.T) {
	root := doneFixture(t)
	if _, err := Claim(root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !locked(root, "ALP-1") {
		t.Fatal("замок не поставлен перед тестом")
	}
	res, err := Done(root, "ALP-1", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if res.Result.From != "in-progress" || res.Result.To != "done" {
		t.Errorf("Result = %q -> %q", res.Result.From, res.Result.To)
	}
	got := read(t, filepath.Join(root, "tasks", "alpha", "one.md"))
	if strings.Contains(got, "agent: claude") {
		t.Errorf("claim не снят:\n%s", got)
	}
	if locked(root, "ALP-1") {
		t.Error("замок не снят")
	}
}

func TestDoneUnparsableTask(t *testing.T) {
	root := doneFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	if err := os.WriteFile(path, []byte("---\nid: ALP-1\nstatus: [broken\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Done(root, "ALP-1", "", "claude")
	if kind, ok := KindOf(err); !ok || kind != KindUnparsable {
		t.Errorf("вид отказа = %v (ok=%v), ожидался KindUnparsable", kind, ok)
	}
}

func TestDonePrintsPromotedList(t *testing.T) {
	// Проверка на уровне cli — что список печатается — в internal/cli/done_test.go.
	// Здесь только гарантия, что Done действительно отдаёт список наружу.
	root := doneFixture(t)
	res, err := Done(root, "ALP-1", "", "claude")
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if len(res.Promoted) == 0 {
		t.Fatal("ожидался непустой список промоута")
	}
}
