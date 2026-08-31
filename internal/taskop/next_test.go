package taskop

import (
	"os"
	"path/filepath"
	"testing"
)

// nextFixture — vault с проектом alpha: россыпь тасок под все правила отбора
// tt next. ALP-1 — единственный чистый кандидат по умолчанию (high, самый
// ранний ready_at), остальные каждая ломает ровно одно условие.
func nextFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"projects/alpha.md": "---\nproject: alpha\nid_prefix: ALP\nnext_id: 12\nrepo:\n---\n",
		// ALP-1 — ready, high, самый ранний ready_at: главный кандидат.
		"tasks/alpha/one.md": "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-08-01\nattempts: 0\nclaim:\n---\n",
		// ALP-2 — тоже ready/high, но ready_at позже: должна проиграть ALP-1.
		"tasks/alpha/two.md": "---\nid: ALP-2\ntitle: вторая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-08-05\nattempts: 0\nclaim:\n---\n",
		// ALP-3 — ready/medium: ниже приоритетом ALP-1 и ALP-2.
		"tasks/alpha/three.md": "---\nid: ALP-3\ntitle: третья\nstatus: ready\nproject: alpha\n" +
			"priority: medium\ncreated: 2026-08-01\nready_at: 2026-07-01\nattempts: 0\nclaim:\n---\n",
		// ALP-4 — ready/high, но с живым блокером ALP-9 (backlog): исключается.
		"tasks/alpha/four.md": "---\nid: ALP-4\ntitle: четвёртая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-07-01\nblocked_by: [ALP-9]\nattempts: 0\nclaim:\n---\n",
		// ALP-5 — ready/high, но занята чужим claim: исключается.
		"tasks/alpha/five.md": "---\nid: ALP-5\ntitle: пятая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-07-01\nattempts: 0\nclaim:\n  agent: чужой\n  host: x\n  branch: y\n  started: 2026-08-01\n---\n",
		// ALP-6 — ready/high, но на неё стоит файловый замок: исключается.
		"tasks/alpha/six.md": "---\nid: ALP-6\ntitle: шестая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-07-01\nattempts: 0\nclaim:\n---\n",
		// ALP-7 — backlog: не agentPickable, исключается.
		"tasks/alpha/seven.md": "---\nid: ALP-7\ntitle: седьмая\nstatus: backlog\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nattempts: 0\nclaim:\n---\n",
		// ALP-8 — hold: не agentPickable, исключается.
		"tasks/alpha/eight.md": "---\nid: ALP-8\ntitle: восьмая\nstatus: hold\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nattempts: 0\nclaim:\n---\n",
		// ALP-9 — блокер ALP-4, живой (backlog).
		"tasks/alpha/nine.md": "---\nid: ALP-9\ntitle: девятая\nstatus: backlog\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nattempts: 0\nclaim:\n---\n",
		// ALP-10 — done: не agentPickable.
		"tasks/alpha/ten.md": "---\nid: ALP-10\ntitle: десятая\nstatus: done\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\ncompleted: 2026-08-02\nattempts: 0\nclaim:\n---\n",
		// ALP-11 — другой проект, чтобы фильтр --project правда фильтровал.
		"tasks/beta/eleven.md": "---\nid: ALP-11\ntitle: одиннадцатая\nstatus: ready\nproject: beta\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-01-01\nattempts: 0\nclaim:\n---\n",
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
	if err := os.MkdirAll(lockDir(root, "ALP-6"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNextPicksHighestPriorityEarliestReadyAt(t *testing.T) {
	root := nextFixture(t)
	task, ok, err := Next(root, NextOptions{Project: "alpha"})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !ok {
		t.Fatal("ожидался кандидат, очередь пуста")
	}
	if task.ID != "ALP-1" {
		t.Errorf("ID = %q, ожидался ALP-1", task.ID)
	}
}

func TestNextIsDeterministicAcrossRuns(t *testing.T) {
	root := nextFixture(t)
	first, _, err := Next(root, NextOptions{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Next(root, NextOptions{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("два прогона дали разные таски: %q и %q", first.ID, second.ID)
	}
}

func TestNextSkipsBlockedClaimedAndLockedTasks(t *testing.T) {
	root := nextFixture(t)
	// Убираем ALP-1 и ALP-2 (лучших кандидатов), чтобы проверить, что
	// следующий по очереди ALP-3 берётся, а не ALP-4/5/6 с нарушениями.
	for _, id := range []string{"one.md", "two.md"} {
		if err := os.Remove(filepath.Join(root, "tasks", "alpha", id)); err != nil {
			t.Fatal(err)
		}
	}
	task, ok, err := Next(root, NextOptions{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ожидался кандидат")
	}
	if task.ID != "ALP-3" {
		t.Errorf("ID = %q, ожидался ALP-3 (ALP-4/5/6 должны быть исключены)", task.ID)
	}
}

func TestNextIgnoresOtherProjects(t *testing.T) {
	root := nextFixture(t)
	task, ok, err := Next(root, NextOptions{Project: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || task.ID != "ALP-11" {
		t.Errorf("ожидался ALP-11 из beta, получили ok=%v id=%q", ok, task.ID)
	}
}

func TestNextEmptyQueueIsNotAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	task, ok, err := Next(root, NextOptions{Project: "empty"})
	if err != nil {
		t.Fatalf("пустая очередь не должна быть ошибкой: %v", err)
	}
	if ok {
		t.Errorf("ожидалась пустая очередь, получили %+v", task)
	}
}

// customSchemaFixture — vault со своей схемой, где agentPickable стоит у
// нестандартного статуса "triaged", а не у "ready". Правило обязано браться
// из схемы, а не быть зашито константой "ready" в коде.
func customSchemaFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	schema := `{
  "version": 1,
  "statuses": [
    { "id": "backlog", "lane": "Backlog" },
    { "id": "triaged", "lane": "Triaged", "agentPickable": true },
    { "id": "ready", "lane": "Ready" },
    { "id": "done", "lane": "Done" }
  ],
  "aliases": {},
  "clearsClaim": ["done"],
  "setsCompleted": ["done"],
  "setsReadyAt": ["triaged"],
  "promoteFrom": "backlog",
  "promoteTo": "triaged"
}`
	files := map[string]string{
		".task-tracker/schema.json": schema,
		"projects/alpha.md":         "---\nproject: alpha\nid_prefix: ALP\nnext_id: 3\nrepo:\n---\n",
		"tasks/alpha/one.md": "---\nid: ALP-1\ntitle: первая\nstatus: triaged\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-08-01\nattempts: 0\nclaim:\n---\n",
		"tasks/alpha/two.md": "---\nid: ALP-2\ntitle: вторая\nstatus: ready\nproject: alpha\n" +
			"priority: high\ncreated: 2026-08-01\nready_at: 2026-08-01\nattempts: 0\nclaim:\n---\n",
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

func TestNextUsesAgentPickableFromSchemaNotHardcoded(t *testing.T) {
	root := customSchemaFixture(t)
	task, ok, err := Next(root, NextOptions{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ожидался кандидат")
	}
	if task.ID != "ALP-1" {
		t.Errorf("ID = %q, ожидался ALP-1 (статус triaged, agentPickable в схеме)", task.ID)
	}
}
