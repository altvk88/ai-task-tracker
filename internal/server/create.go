package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/alkulagin-creator/tt/internal/taskop"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// createTaskRequest — тело POST /api/task.
type createTaskRequest struct {
	Project   string   `json:"project"`
	Title     string   `json:"title"`
	Priority  string   `json:"priority"`
	Effort    string   `json:"effort"`
	Spec      string   `json:"spec"`
	DependsOn []string `json:"dependsOn"`
}

// createTaskResponse — созданная таска плюс предупреждения про
// незаведённые зависимости (см. taskop.NewResult.Warnings) — это не отказ,
// таска-блокер может появиться следующим же вызовом.
type createTaskResponse struct {
	apiTask
	Warnings []string `json:"warnings,omitempty"`
}

// maxCreateTaskBody — как maxFieldBody: несколько коротких строк и список
// зависимостей, крупное тело — не наш клиент.
const maxCreateTaskBody = 8 << 10

// handleCreateTask — POST /api/task: создание таски. ID выдаёт taskop.New
// под тем же замком, что и `tt new` / plan-to-tasks — сервер id не изобретает
// и не привносит собственных правил гейтинга статуса.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только POST")
		return
	}

	var req createTaskRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxCreateTaskBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `тело запроса ожидается в виде {"project":"...","title":"...","priority":"...","effort":"...","spec":"...","dependsOn":[...]}`)
		return
	}
	if req.Project == "" || req.Title == "" {
		writeError(w, http.StatusBadRequest, "не указаны обязательные поля project и title")
		return
	}

	res, err := taskop.New(s.vaultDir, taskop.NewOptions{
		Project:   req.Project,
		Title:     req.Title,
		Priority:  req.Priority,
		Effort:    req.Effort,
		Spec:      req.Spec,
		DependsOn: req.DependsOn,
	})
	if err != nil {
		writeError(w, statusCodeFor(err), err.Error())
		return
	}

	// taskop.New возвращает только ID/Path/Status/Warnings, а не полную
	// таску — читаем файл сами, как это уже делает handleTask, чтобы отдать
	// клиенту тот же формат, что и остальные ручки.
	raw, readErr := os.ReadFile(res.Path)
	if readErr != nil {
		writeError(w, http.StatusInternalServerError, "таска "+res.ID+" создана, но не удалось прочитать файл для ответа")
		return
	}
	task, parseErr := vault.Parse(raw)
	task.Path = res.Path
	if parseErr != nil {
		task.ParseErr = parseErr.Error()
	}
	// Индекс обновляем тем же принципом, что и в respondWithTask: не ждём
	// fsnotify, чтобы созданная таска сразу была видна в /api/snapshot.
	s.ix.Apply(res.Path)

	status, _ := s.ix.Schema().Normalize(task.Status)
	out := toAPITask(task, status)
	out.Version = fileVersion(res.Path)
	writeJSON(w, http.StatusCreated, createTaskResponse{apiTask: out, Warnings: res.Warnings})
}
