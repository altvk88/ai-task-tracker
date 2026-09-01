// Package server — HTTP-обвязка над internal/index: снимок vault и таска по
// API. Пакет не разбирает markdown и не знает про фронтматтер — за это
// отвечают internal/vault и internal/model.
package server

import (
	"net/http"
	"time"

	"github.com/alkulagin-creator/tt/internal/index"
)

// Options — параметры сервера. Токен и адрес слушателя появятся в следующих
// задачах, когда до них дойдёт дело (`tt serve`).
type Options struct {
	// HeartbeatInterval — период SSE-пульса на /api/events. Ноль или
	// отрицательное значение — использовать defaultHeartbeat. Настраиваемо,
	// иначе тесты ждали бы реальные 30 секунд.
	HeartbeatInterval time.Duration

	// Agent — имя писателя, от которого сервер меняет статусы. Пусто —
	// defaultAgent. Оно сравнивается с claim таски, поэтому веб не должен
	// притворяться bash-агентом: чужую занятую таску забирать нельзя.
	Agent string

	// Token — токен на запись для запросов не с loopback-адреса (см.
	// auth.go). Пусто — проверка токена отключена целиком: так живёт
	// большинство тестов пакета, которым авторизация не по теме.
	Token string
}

// defaultAgent — под этим именем пишет веб-клиент, если не задано иное.
const defaultAgent = "web"

// Server отдаёт снимок vault и отдельные таски по HTTP.
type Server struct {
	ix       *index.Index
	vaultDir string
	opts     Options
}

// New создаёт сервер над уже построенным индексом. vaultDir нужен для чтения
// тела таски по её пути — сам индекс тела не хранит.
func New(ix *index.Index, vaultDir string, opts Options) *Server {
	return &Server{ix: ix, vaultDir: vaultDir, opts: opts}
}

// Handler собирает маршруты в http.Handler, готовый для httptest или
// http.ListenAndServe. Методы проверяются вручную в каждом обработчике, а не
// через автоматический 405 у http.ServeMux — тот отвечает HTML, а контракт
// API требует JSON на любую ошибку.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	mux.HandleFunc("/api/task", s.handleCreateTask)
	mux.HandleFunc("/api/task/{id}", s.handleTask)
	mux.HandleFunc("/api/task/{id}/status", s.handleSetStatus)
	mux.HandleFunc("/api/task/{id}/field", s.handleSetField)
	mux.HandleFunc("/api/task/{id}/body", s.handleSetBody)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/", handleAPINotFound)
	mux.Handle("/", staticHandler())
	return s.authMiddleware(mux)
}

// handleAPINotFound — неизвестный маршрут внутри /api/*. Контракт API требует
// JSON на любую ошибку, в отличие от статики на "/", которая отдаёт HTML.
func handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "маршрут не найден")
}
