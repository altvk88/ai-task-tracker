package model

import (
	"strings"
	"testing"
)

func tasks(list ...Task) map[string]Task {
	m := map[string]Task{}
	for _, t := range list {
		m[t.ID] = t
	}
	return m
}

func TestUnresolvedBlockers(t *testing.T) {
	all := tasks(
		Task{ID: "A-1", Status: "done"},
		Task{ID: "A-2", Status: "in-progress"},
		Task{ID: "A-3", Status: "cancelled"},
	)

	cases := []struct {
		name string
		task Task
		want []string
	}{
		{"без блокеров", Task{ID: "X", BlockedBy: nil}, nil},
		{"блокер закрыт", Task{ID: "X", BlockedBy: []string{"A-1"}}, nil},
		{"блокер отменён — тоже не держит", Task{ID: "X", BlockedBy: []string{"A-3"}}, nil},
		{"блокер в работе", Task{ID: "X", BlockedBy: []string{"A-2"}}, []string{"A-2"}},
		{"смешанный", Task{ID: "X", BlockedBy: []string{"A-1", "A-2"}}, []string{"A-2"}},
		{"несуществующий блокер держит", Task{ID: "X", BlockedBy: []string{"A-99"}}, []string{"A-99"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := UnresolvedBlockers(c.task, all)
			if len(got) != len(c.want) {
				t.Fatalf("получено %v, ожидалось %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("получено %v, ожидалось %v", got, c.want)
				}
			}
		})
	}
}

func TestPromotable(t *testing.T) {
	s, _ := DefaultSchema()
	all := []Task{
		{ID: "A-1", Status: "done"},
		{ID: "A-2", Status: "backlog", BlockedBy: []string{"A-1"}},        // готова
		{ID: "A-3", Status: "backlog", BlockedBy: []string{"A-1", "A-4"}}, // держит A-4
		{ID: "A-4", Status: "ready"},
		{ID: "A-5", Status: "backlog"},                             // без блокеров — готова
		{ID: "A-6", Status: "blocked", BlockedBy: []string{"A-1"}}, // blocked не промоутится
		{ID: "A-7", Status: "hold", BlockedBy: []string{"A-1"}},    // hold не промоутится
	}
	got := Promotable(s, all)
	want := []string{"A-2", "A-5"}
	if len(got) != len(want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("получено %v, ожидалось %v", got, want)
		}
	}
}

func TestCheckTransition(t *testing.T) {
	s, _ := DefaultSchema()
	all := tasks(
		Task{ID: "A-1", Status: "in-progress"},
		Task{ID: "A-2", Status: "done"},
	)

	t.Run("неизвестный статус отклоняется", func(t *testing.T) {
		if err := CheckTransition(s, Task{ID: "X", Status: "ready"}, "выдумка", all, "claude"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
	t.Run("нельзя брать в работу с живым блокером", func(t *testing.T) {
		task := Task{ID: "X", Status: "ready", BlockedBy: []string{"A-1"}}
		err := CheckTransition(s, task, "in-progress", all, "claude")
		if err == nil {
			t.Fatal("ожидалась ошибка про блокер A-1")
		}
		if !strings.Contains(err.Error(), "A-1") {
			t.Errorf("ошибка обязана называть блокер, получено: %v", err)
		}
	})
	t.Run("с закрытым блокером можно", func(t *testing.T) {
		task := Task{ID: "X", Status: "ready", BlockedBy: []string{"A-2"}}
		if err := CheckTransition(s, task, "in-progress", all, "claude"); err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
	})
	t.Run("историческое написание целевого статуса принимается", func(t *testing.T) {
		task := Task{ID: "X", Status: "ready"}
		if err := CheckTransition(s, task, "in_progress", all, "claude"); err != nil {
			t.Fatalf("in_progress обязан нормализоваться: %v", err)
		}
	})
	t.Run("нельзя отобрать чужой claim", func(t *testing.T) {
		task := Task{ID: "X", Status: "in-progress", Claim: &Claim{Agent: "other"}}
		if err := CheckTransition(s, task, "in-progress", all, "claude"); err == nil {
			t.Fatal("ожидалась ошибка про чужой claim")
		}
	})
	t.Run("свой claim не мешает", func(t *testing.T) {
		task := Task{ID: "X", Status: "in-progress", Claim: &Claim{Agent: "claude"}}
		if err := CheckTransition(s, task, "in-progress", all, "claude"); err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
	})
	t.Run("уход из in-progress с чужим claim разрешён человеку", func(t *testing.T) {
		task := Task{ID: "X", Status: "in-progress", Claim: &Claim{Agent: "other"}}
		if err := CheckTransition(s, task, "ready", all, "claude"); err != nil {
			t.Fatalf("перетаскивание карточки из работы обязано быть разрешено: %v", err)
		}
	})
	t.Run("блокер не мешает уйти в backlog", func(t *testing.T) {
		task := Task{ID: "X", Status: "ready", BlockedBy: []string{"A-1"}}
		if err := CheckTransition(s, task, "backlog", all, "claude"); err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
	})
	t.Run("скалярный claim занимает таску неизвестно кем", func(t *testing.T) {
		task := Task{ID: "X", Status: "in-progress", Claim: &Claim{Raw: "claude 2026-08-04"}}
		err := CheckTransition(s, task, "in-progress", all, "claude")
		if err == nil {
			t.Fatal("ожидалась ошибка про скалярный claim")
		}
		if !strings.Contains(err.Error(), "claude 2026-08-04") {
			t.Errorf("ошибка обязана показывать Raw, получено: %v", err)
		}
	})
}
