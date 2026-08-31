package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alkulagin-creator/tt/internal/index"
	"github.com/alkulagin-creator/tt/internal/model"
)

// defaultHeartbeat — период SSE-пульса, если Options.HeartbeatInterval не
// задан. Без пульса простаивающее соединение рвут прокси и браузеры, причём
// клиент узнаёт об этом не сразу.
const defaultHeartbeat = 30 * time.Second

// flusher — минимум, нужный serveSSE для потоковой записи. Отдельный
// интерфейс (а не сразу http.ResponseWriter) даёт тестировать формирование
// событий на httptest.ResponseRecorder и голом канале, без реального
// HTTP-соединения.
type flusher interface {
	io.Writer
	http.Flusher
}

// sseChange — тело события "change": вид изменения плюс таска в том же
// формате, что и в /api/snapshot, — этого клиенту достаточно, чтобы обновить
// доску без повторного запроса снимка.
type sseChange struct {
	ID   string  `json:"id"`
	Kind string  `json:"kind"`
	Task apiTask `json:"task"`
}

func changeKindString(k index.ChangeKind) string {
	switch k {
	case index.Added:
		return "added"
	case index.Removed:
		return "removed"
	default:
		return "updated"
	}
}

func toSSEChange(c index.Change, schema *model.Schema) sseChange {
	status, _ := schema.Normalize(c.Task.Status)
	return sseChange{ID: c.ID, Kind: changeKindString(c.Kind), Task: toAPITask(c.Task, status)}
}

// writeSSE пишет одно SSE-событие: имя события и JSON-тело одной строкой.
func writeSSE(w io.Writer, event string, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		body = []byte("{}")
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, body)
}

// handleEvents — GET /api/events: поток изменений индекса в формате SSE.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "поддерживается только GET")
		return
	}

	fw, ok := w.(flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "сервер не поддерживает потоковую отдачу")
		return
	}

	ch, unsubscribe := s.ix.Subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fw.Flush()

	serveSSE(r.Context(), fw, ch, s.ix.Schema(), s.opts.HeartbeatInterval)
}

// serveSSE — цикл раздачи одному подписчику: изменения индекса, пульс и
// корректная реакция на отставание. Вынесен из handleEvents отдельной
// функцией, не завязанной на *http.Request/ResponseWriter, чтобы тестировать
// формат событий и обработку закрытого канала без реального HTTP-соединения.
func serveSSE(ctx context.Context, w flusher, ch <-chan index.Change, schema *model.Schema, heartbeat time.Duration) {
	if heartbeat <= 0 {
		heartbeat = defaultHeartbeat
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-ch:
			if !ok {
				// Подписчик отстал: index закрыл канал (см. index.Subscribe).
				// Молчать нельзя — клиент решит, что просто ничего не
				// изменилось, хотя его состояние уже устарело.
				writeSSE(w, "resync", struct{}{})
				w.Flush()
				return
			}
			writeSSE(w, "change", toSSEChange(c, schema))
			w.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			w.Flush()
		}
	}
}
