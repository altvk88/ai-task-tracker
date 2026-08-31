package model

import "fmt"

// resolvedStatuses — статусы, в которых блокер больше никого не держит.
var resolvedStatuses = map[string]bool{"done": true, "cancelled": true}

// UnresolvedBlockers возвращает ID блокеров, которые всё ещё держат таску.
// Несуществующий блокер считается держащим: это ошибка данных, и молча
// разблокировать таску из-за опечатки в ID нельзя.
func UnresolvedBlockers(t Task, byID map[string]Task) []string {
	var out []string
	for _, id := range t.BlockedBy {
		if id == "" {
			continue
		}
		blocker, ok := byID[id]
		if !ok || !resolvedStatuses[blocker.Status] {
			out = append(out, id)
		}
	}
	return out
}

// Promotable отдаёт ID тасок, которые пора перевести из promoteFrom в promoteTo:
// все их блокеры закрыты. Порядок — как во входном срезе, чтобы вывод был
// предсказуемым.
func Promotable(s *Schema, all []Task) []string {
	byID := make(map[string]Task, len(all))
	for _, t := range all {
		if t.ID != "" {
			byID[t.ID] = t
		}
	}
	var out []string
	for _, t := range all {
		if t.Status != s.PromoteFrom {
			continue
		}
		if len(UnresolvedBlockers(t, byID)) == 0 {
			out = append(out, t.ID)
		}
	}
	return out
}

// CheckTransition проверяет два условия, которые действительно стоит стеречь:
// нельзя взять в работу таску с живыми блокерами и нельзя отобрать чужой claim.
// Полной матрицы переходов сознательно нет: перетаскивание карточки между
// остальными лейнами законно, и доска не должна спорить с человеком.
func CheckTransition(s *Schema, t Task, to string, byID map[string]Task, agent string) error {
	canon, known := s.Normalize(to)
	if !known {
		return fmt.Errorf("неизвестный статус %q", to)
	}
	if canon != "in-progress" {
		return nil
	}
	if blockers := UnresolvedBlockers(t, byID); len(blockers) > 0 {
		return fmt.Errorf("таска %s заблокирована: %v", t.ID, blockers)
	}
	if t.Claimed() {
		// Скалярный claim (Raw заполнен, Agent пуст) — историческая запись
		// вида "claude 2026-08-04", из которой нельзя достоверно извлечь
		// владельца. Такую таску нельзя молча считать своей: если Agent
		// пуст, но Raw не пуст, значит claim принадлежит неизвестно кому.
		if t.Claim.Agent == "" {
			return fmt.Errorf("таска %s занята (нераспознанный claim: %q)", t.ID, t.Claim.Raw)
		}
		if t.Claim.Agent != agent {
			return fmt.Errorf("таска %s занята агентом %s", t.ID, t.Claim.Agent)
		}
	}
	return nil
}
