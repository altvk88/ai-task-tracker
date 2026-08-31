package taskop

import (
	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// DoneResult — итог закрытия: сама таска (как Result у Set/Claim) плюс ID
// тасок, которых закрытие разблокировало и перевело в ready.
type DoneResult struct {
	Result   Result
	Promoted []string
}

// Done закрывает таску: статус done, completed сегодняшней датой, claim и
// замок сняты, result записан, если передан. Дополнительно промоутит
// зависимые таски — замена ручному разбору blocked_by после каждого /done.
//
// Промоут нарочно уже сузили до тасок, у которых закрываемая id стояла
// блокером: model.Promotable отдаёт всех кандидатов в promoteFrom без живых
// блокеров вообще, а в vault обычно уже есть backlog-таски без единого
// блокера — они promotable в любой момент независимо от того, что сейчас
// закрывают. Проверка на копии живого vault (1301 таска) это подтвердила:
// из 4 текущих кандидатов на промоут 1 не имеет блокеров вовсе — «просто
// закрыть первую попавшуюся таску» и получить в выводе чужой промоут было бы
// сюрпризом для человека, которому нужно видеть последствия именно своего
// действия.
func Done(vaultDir, id, resultText, agent string) (DoneResult, error) {
	schema, byID, task, err := locate(vaultDir, id)
	if err != nil {
		return DoneResult{}, err
	}
	canon, known := schema.Normalize("done")
	if !known {
		return DoneResult{}, failf(KindBadValue, "в схеме нет статуса done")
	}
	if err := model.CheckTransition(schema, task, canon, byID, agent); err != nil {
		return DoneResult{}, failf(KindRejected, "%w", err)
	}
	cur, _ := schema.Normalize(task.Status)
	alreadyDone := cur == canon

	if err := applyStatus(schema, task, canon); err != nil {
		return DoneResult{}, failf(KindWrite, "%w", err)
	}
	unlockDir(vaultDir, id)

	if resultText != "" {
		if err := vault.SetField(task.Path, "result", resultText); err != nil {
			return DoneResult{}, failf(KindWrite, "%w", err)
		}
	}

	var promoted []string
	if !alreadyDone {
		promoted, err = promoteDependents(vaultDir, id)
		if err != nil {
			return DoneResult{}, err
		}
	}

	res, err := reread(task, task.Status, canon)
	if err != nil {
		return DoneResult{}, err
	}
	return DoneResult{Result: res, Promoted: promoted}, nil
}

// promoteDependents переводит в ready тасок из model.Promotable, но только
// тех, у кого closedID числится среди блокеров — иначе закрытие одной таски
// молча промоутило бы посторонние backlog-таски без единого блокера, которые
// уже подходят под promoteFrom независимо от этого закрытия.
func promoteDependents(vaultDir, closedID string) ([]string, error) {
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return nil, failf(KindWrite, "%w", err)
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return nil, failf(KindWrite, "%w", err)
	}
	byID := vault.ByID(tasks)

	var promoted []string
	for _, candidateID := range model.Promotable(schema, tasks) {
		candidate := byID[candidateID]
		blockedByClosed := false
		for _, b := range candidate.BlockedBy {
			if b == closedID {
				blockedByClosed = true
				break
			}
		}
		if !blockedByClosed {
			continue
		}
		if err := applyStatus(schema, candidate, schema.PromoteTo); err != nil {
			return promoted, failf(KindWrite, "%w", err)
		}
		promoted = append(promoted, candidateID)
	}
	return promoted, nil
}
