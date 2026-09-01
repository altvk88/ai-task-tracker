package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// fileVersion — признак версии таски для клиента: см. vault.Version.
// Ошибка Stat (файл только что удалили) не повод падать 500 на снимке —
// пустая версия просто не даст записи пройти конфликт-проверку у той
// таски, зато остальные 1296 строк снимка не пострадают.
func fileVersion(path string) string {
	v, err := vault.Version(path)
	if err != nil {
		return ""
	}
	return v
}

// apiTask — таска в снимке. Имена и состав полей — как у listRow из
// internal/cli/list.go: снимок читают и веб-клиент, и агенты, привыкшие к
// `tt list --json`, расхождение в именах между ними того не стоит.
//
// Effort, Completed, BlockedBy и Claim добавлены поверх исходного набора
// ради веб-доски (TT-033): карточке нужен эффорт и метка занятости, а
// скрытию старых закрытых тасок — дата завершения. Раздувает снимок, но
// заводить для этого второй эндпоинт незачем — тасок всё равно 1297.
type apiTask struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Project   string    `json:"project"`
	Priority  string    `json:"priority"`
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	ParseErr  string    `json:"parseError,omitempty"`
	Effort    string    `json:"effort,omitempty"`
	Completed string    `json:"completed,omitempty"`
	BlockedBy []string  `json:"blockedBy,omitempty"`
	Claim     *apiClaim `json:"claim,omitempty"`

	// Version — признак версии файла на момент ответа (см. vault.Version).
	// Клиент обязан вернуть его как baseVersion в POST .../field и
	// .../body — так конкурентная правка ловится конфликтом, а не тихо
	// перезаписывается.
	Version string `json:"version,omitempty"`
}

// summary — сводка по снимку.
type summary struct {
	Total  int `json:"total"`
	Broken int `json:"broken"`
}

// snapshotResponse — тело ответа GET /api/snapshot.
type snapshotResponse struct {
	Tasks   []apiTask     `json:"tasks"`
	Schema  *model.Schema `json:"schema"`
	Summary summary       `json:"summary"`
}

// apiClaim — блок claim в детальном ответе таски.
type apiClaim struct {
	Agent   string `json:"agent,omitempty"`
	Host    string `json:"host,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Started string `json:"started,omitempty"`
	Raw     string `json:"raw,omitempty"`
}

// taskDetail — тело ответа GET /api/task/{id}: та же таска, что и в снимке,
// плюс остальные поля фронтматтера (в снимок они не идут, чтобы не раздувать
// его — 1297 тасок в браузер и так едут одним куском) и тело markdown.
type taskDetail struct {
	apiTask
	Due      string   `json:"due,omitempty"`
	Created  string   `json:"created,omitempty"`
	ReadyAt  string   `json:"readyAt,omitempty"`
	Attempts int      `json:"attempts,omitempty"`
	Verify   []string `json:"verify,omitempty"`
	Spec     string   `json:"spec,omitempty"`
	Result   string   `json:"result,omitempty"`
	Body     string   `json:"body"`
}

// errorResponse — единый формат ошибки API.
type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func toAPITask(t model.Task, status string) apiTask {
	return apiTask{
		ID: t.ID, Status: status, Project: t.Project,
		Priority: t.Priority, Title: t.Title, Path: t.Path, ParseErr: t.ParseErr,
		Effort: t.Effort, Completed: t.Completed, BlockedBy: t.BlockedBy,
		Claim: toAPIClaim(t.Claim),
	}
}

func toAPIClaim(c *model.Claim) *apiClaim {
	if c == nil {
		return nil
	}
	return &apiClaim{Agent: c.Agent, Host: c.Host, Branch: c.Branch, Started: c.Started, Raw: c.Raw}
}

// handleSnapshot — GET /api/snapshot: все таски, схема флоу и сводка.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только GET")
		return
	}

	tasks := s.ix.Snapshot()
	schema := s.ix.Schema()

	rows := make([]apiTask, 0, len(tasks))
	broken := 0
	for _, t := range tasks {
		status, _ := schema.Normalize(t.Status)
		if t.ParseErr != "" {
			broken++
		}
		row := toAPITask(t, status)
		row.Version = fileVersion(t.Path)
		rows = append(rows, row)
	}

	writeJSON(w, http.StatusOK, snapshotResponse{
		Tasks:  rows,
		Schema: schema,
		Summary: summary{
			Total:  len(tasks),
			Broken: broken,
		},
	})
}

// handleTask — GET /api/task/{id}: одна таска плюс тело markdown. Тело
// читается лениво прямо здесь, а не хранится в индексе: держать в памяти
// 1297 тел незачем, снимок из-за этого раздулся бы в разы.
func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только GET")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "не указан id таски")
		return
	}

	task, ok := s.ix.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("таска %q не найдена", id))
		return
	}

	raw, err := os.ReadFile(task.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать файл таски")
		return
	}
	body, err := vault.Body(raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось выделить тело таски")
		return
	}

	schema := s.ix.Schema()
	status, _ := schema.Normalize(task.Status)

	apiT := toAPITask(task, status)
	apiT.Version = fileVersion(task.Path)
	writeJSON(w, http.StatusOK, taskDetail{
		apiTask:  apiT,
		Due:      task.Due,
		Created:  task.Created,
		ReadyAt:  task.ReadyAt,
		Attempts: task.Attempts,
		Verify:   []string(task.Verify),
		Spec:     task.Spec,
		Result:   task.Result,
		Body:     body,
	})
}
