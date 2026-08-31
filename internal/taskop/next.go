package taskop

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// NextOptions — фильтры выбора следующей таски.
type NextOptions struct {
	Project string
}

// priorityWeight — порядок приоритетов для сортировки: высокий раньше низкого,
// пустой приоритет — в самом конце, а не в начале (как было бы при пустой
// строке в обычном сравнении).
var priorityWeight = map[string]int{"high": 0, "medium": 1, "low": 2}

func weightOf(priority string) int {
	if w, ok := priorityWeight[priority]; ok {
		return w
	}
	return len(priorityWeight)
}

// Next выбирает одну таску для агента: ничего не пишет, только советует.
// Кандидаты — таски проекта в статусе, помеченном agentPickable в схеме, без
// живых блокеров, без чужого claim и без файлового замка. Пустая очередь —
// не ошибка: второй возврат сообщает об этом явно.
//
// Сортировка — приоритет, затем ready_at по возрастанию (кто дольше ждёт),
// затем ID: два прогона на одних данных обязаны дать одну и ту же таску.
func Next(vaultDir string, opt NextOptions) (model.Task, bool, error) {
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return model.Task{}, false, failf(KindWrite, "%w", err)
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return model.Task{}, false, failf(KindWrite, "%w", err)
	}
	byID := vault.ByID(tasks)

	var candidates []model.Task
	for _, t := range tasks {
		if t.ParseErr != "" {
			continue
		}
		if opt.Project != "" && t.Project != opt.Project {
			continue
		}
		canon, known := schema.Normalize(t.Status)
		if !known || !schema.AgentPickable(canon) {
			continue
		}
		if len(model.UnresolvedBlockers(t, byID)) > 0 {
			continue
		}
		if t.Claimed() {
			continue
		}
		if hasLock(vaultDir, t.ID) {
			continue
		}
		candidates = append(candidates, t)
	}
	if len(candidates) == 0 {
		return model.Task{}, false, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if wa, wb := weightOf(a.Priority), weightOf(b.Priority); wa != wb {
			return wa < wb
		}
		// Таска без ready_at не была помечена готовой явно — не даём ей
		// обгонять тех, у кого дата есть.
		if ra, rb := a.ReadyAt == "", b.ReadyAt == ""; ra != rb {
			return rb
		}
		if a.ReadyAt != b.ReadyAt {
			return a.ReadyAt < b.ReadyAt
		}
		return a.ID < b.ID
	})
	return candidates[0], true, nil
}

// hasLock сообщает, стоит ли на таске файловый замок <vault>/.locks/<ID>.lock.
// Отдельная проверка от Claimed() нужна: замок и блок claim во фронтматтере —
// два независимых признака «занято», и залипшая таска может нести только один
// из них.
func hasLock(vaultDir, id string) bool {
	_, err := os.Stat(filepath.Join(vaultDir, ".locks", id+".lock"))
	return err == nil
}
