package model

import "testing"

func TestDefaultSchemaNormalize(t *testing.T) {
	s, err := DefaultSchema()
	if err != nil {
		t.Fatalf("дефолтная схема не читается: %v", err)
	}
	cases := []struct {
		in    string
		want  string
		known bool
	}{
		{"ready", "ready", true},
		{"in-progress", "in-progress", true},
		{"in_progress", "in-progress", true}, // историческое написание
		{"needs_input", "needs-input", true},
		{"canceled", "cancelled", true}, // US-написание
		{"on-hold", "hold", true},
		{"ONHOLD", "hold", true},     // регистр не важен
		{"  ready  ", "ready", true}, // пробелы из фронтматтера
		{"выдумка", "выдумка", false},
	}
	for _, c := range cases {
		got, known := s.Normalize(c.in)
		if got != c.want || known != c.known {
			t.Errorf("Normalize(%q) = (%q, %v), ожидалось (%q, %v)", c.in, got, known, c.want, c.known)
		}
	}
}

func TestDefaultSchemaLanes(t *testing.T) {
	s, err := DefaultSchema()
	if err != nil {
		t.Fatalf("дефолтная схема не читается: %v", err)
	}
	if got := s.Lane("in-progress"); got != "In Progress" {
		t.Errorf("Lane(in-progress) = %q", got)
	}
	if got := s.Lane("cancelled"); got != "Canceled" {
		t.Errorf("Lane(cancelled) = %q", got)
	}
	// Порядок лейнов — это порядок статусов в схеме, доски обеих реализаций
	// обязаны рисовать его одинаково.
	want := []string{"backlog", "ready", "in-progress", "needs-input", "blocked",
		"hold", "failed", "done", "cancelled", "todo"}
	if len(s.Statuses) != len(want) {
		t.Fatalf("статусов %d, ожидалось %d", len(s.Statuses), len(want))
	}
	for i, w := range want {
		if s.Statuses[i].ID != w {
			t.Errorf("статус #%d = %q, ожидался %q", i, s.Statuses[i].ID, w)
		}
	}
}

func TestAgentPickable(t *testing.T) {
	s, _ := DefaultSchema()
	if !s.AgentPickable("ready") {
		t.Error("ready обязан быть доступен агенту")
	}
	for _, st := range []string{"hold", "blocked", "backlog", "done", "todo"} {
		if s.AgentPickable(st) {
			t.Errorf("%s не должен подхватываться агентом автоматически", st)
		}
	}
}
