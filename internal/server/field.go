package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/taskop"
)

// fieldRequest — тело POST /api/task/{id}/field.
type fieldRequest struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	BaseVersion string `json:"baseVersion"`
}

// maxFieldBody — как maxStatusBody: поле короткое, крупное тело — не наш клиент.
const maxFieldBody = 8 << 10

// handleSetField — POST /api/task/{id}/field: правка произвольного поля
// фронтматтера через taskop.SetIfVersion. Статус сюда не пускаем: для него
// есть отдельный маршрут /status с локом и побочными полями — держать два
// пути к одному и тому же поведению того не стоит.
//
// Правил валидации здесь нет: белый список ключей, нормализацию и защиту от
// конкурентной правки делает taskop, обработчик только разбирает запрос и
// переводит вид отказа в код ответа — тот же принцип, что и у handleSetStatus.
func (s *Server) handleSetField(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только POST")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "не указан id таски")
		return
	}

	var req fieldRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxFieldBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `тело запроса ожидается в виде {"key":"...","value":"...","baseVersion":"..."}`)
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "не указан key")
		return
	}
	if req.Key == "status" {
		writeError(w, http.StatusBadRequest, "для смены статуса используйте POST /api/task/{id}/status")
		return
	}
	if req.BaseVersion == "" {
		writeError(w, http.StatusBadRequest, "не указан baseVersion — сначала прочитайте таску через GET /api/task/{id}")
		return
	}

	res, err := taskop.SetIfVersion(s.vaultDir, id, req.Key, req.Value, s.agent(), req.BaseVersion)
	if err != nil {
		writeError(w, statusCodeFor(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, s.respondWithTask(res.Task))
}

// respondWithTask обновляет индекс результатом записи и собирает ответ с
// актуальным статусом и свежей версией файла — общий хвост
// handleSetStatus/handleSetField/handleSetBody. Индекс обновляется синхронно
// с ответом, не дожидаясь fsnotify: у наблюдателя дебаунс 50 мс, и ответ 200
// успел бы уйти раньше, чем изменение стало видно в /api/snapshot. Apply
// идемпотентен, так что повторное применение того же изменения от fsnotify
// следом ничего не портит.
func (s *Server) respondWithTask(task model.Task) apiTask {
	result := task
	if change, changed := s.ix.Apply(task.Path); changed {
		result = change.Task
	}
	status, _ := s.ix.Schema().Normalize(result.Status)
	out := toAPITask(result, status)
	out.Version = fileVersion(result.Path)
	return out
}
