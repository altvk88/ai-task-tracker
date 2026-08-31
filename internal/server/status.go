package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// statusRequest — тело POST /api/task/{id}/status.
type statusRequest struct {
	To string `json:"to"`
}

// maxStatusBody — потолок на тело запроса. Оно состоит из одного короткого
// поля; всё, что крупнее, — не наш клиент, и читать это целиком незачем.
const maxStatusBody = 4 << 10

// handleSetStatus — POST /api/task/{id}/status: смена статуса таски.
//
// Правил здесь нет ни одного: их целиком выполняет taskop.Set, тот же, что
// стоит за `tt set`. Задача обработчика — разобрать запрос, перевести вид
// отказа в код ответа и обновить индекс.
func (s *Server) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только POST")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "не указан id таски")
		return
	}

	var req statusRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxStatusBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "тело запроса ожидается в виде {\"to\":\"<статус>\"}")
		return
	}
	if req.To == "" {
		writeError(w, http.StatusBadRequest, "не указан целевой статус")
		return
	}

	res, err := taskop.Set(s.vaultDir, id, "status", req.To, s.agent())
	if err != nil {
		writeError(w, statusCodeFor(err), err.Error())
		return
	}

	// Индекс обновляем сами, не дожидаясь fsnotify: у наблюдателя дебаунс
	// 50 мс, и ответ 200 успел бы уйти раньше, чем изменение стало видно в
	// /api/snapshot. Apply идемпотентен — когда следом придёт событие от
	// fsnotify, содержимое уже совпадёт, изменения не будет и второго
	// SSE-события подписчики не получат.
	task := res.Task
	if change, changed := s.ix.Apply(res.Task.Path); changed {
		task = change.Task
	}

	status, _ := s.ix.Schema().Normalize(task.Status)
	writeJSON(w, http.StatusOK, toAPITask(task, status))
}

// agent — под чьим именем сервер claim'ит таски. Веб-клиент — отдельный
// писатель наравне с bash-агентами: таску, занятую кем-то другим, он забрать
// не сможет, и это правильно.
func (s *Server) agent() string {
	if s.opts.Agent != "" {
		return s.opts.Agent
	}
	return defaultAgent
}

// statusCodeFor переводит вид отказа записи в код HTTP.
func statusCodeFor(err error) int {
	kind, ok := taskop.KindOf(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch kind {
	case taskop.KindNotFound:
		return http.StatusNotFound
	case taskop.KindUnparsable:
		return http.StatusUnprocessableEntity
	case taskop.KindBadValue:
		return http.StatusBadRequest
	case taskop.KindRejected:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
