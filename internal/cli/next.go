package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// nextRow — вид одной таски в JSON-выводе tt next. Пересекается с listRow не
// целиком: агенту нужен ready_at, чтобы видеть, почему выбрана именно эта
// таска, а не просто её место в общем списке.
type nextRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Project  string `json:"project"`
	Priority string `json:"priority"`
	ReadyAt  string `json:"ready_at"`
	Path     string `json:"path"`
}

// Next печатает следующую таску, которую агенту стоит взять в проекте, или
// сообщает о пустой очереди. Команда только советует: ни замка, ни claim она
// не ставит — за этим отдельно идёт tt claim.
func Next(w io.Writer, vaultDir, project string, asJSON bool) error {
	task, ok, err := taskop.Next(vaultDir, taskop.NextOptions{Project: project})
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if !ok {
			// null, а не {} или пустой объект: агенту, который десериализует
			// ответ в структуру, легче отличить «ничего нет» по nil-значению,
			// чем гадать по нулевым полям пустой таски.
			return enc.Encode(nil)
		}
		return enc.Encode(nextRow{
			ID: task.ID, Title: task.Title, Status: task.Status,
			Project: task.Project, Priority: task.Priority,
			ReadyAt: task.ReadyAt, Path: task.Path,
		})
	}

	if !ok {
		fmt.Fprintf(w, "в проекте %s нечего брать\n", project)
		return nil
	}
	fmt.Fprintf(w, "%s [%s] %s\n", task.ID, task.Priority, task.Title)
	return nil
}
