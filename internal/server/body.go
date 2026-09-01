package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// bodyRequest — тело POST /api/task/{id}/body.
type bodyRequest struct {
	Body        string `json:"body"`
	BaseVersion string `json:"baseVersion"`
}

// maxTaskBodySize — потолок на markdown-тело таски. Тасок в vault 1297,
// самая тяжёлая из них весит несколько КБ вместе с фронтматтером; 256 КиБ —
// щедрый запас, а не бытовой лимит, который панель могла бы упереться.
const maxTaskBodySize = 256 << 10

// handleSetBody — POST /api/task/{id}/body: запись markdown-тела таски
// (всё после фронтматтера) через taskop.SetBody.
//
// Правил здесь нет: существование и разбираемость таски проверяет locate
// внутри taskop, конкурентную правку — общая с /field проверка версии.
// Обработчик разбирает запрос и переводит вид отказа в код ответа.
func (s *Server) handleSetBody(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только POST")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "не указан id таски")
		return
	}

	var req bodyRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxTaskBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `тело запроса ожидается в виде {"body":"...","baseVersion":"..."}`)
		return
	}
	if req.BaseVersion == "" {
		writeError(w, http.StatusBadRequest, "не указан baseVersion — сначала прочитайте таску через GET /api/task/{id}")
		return
	}

	res, err := taskop.SetBody(s.vaultDir, id, req.Body, req.BaseVersion)
	if err != nil {
		writeError(w, statusCodeFor(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, s.respondWithTask(res.Task))
}
