// Package server — HTTP-обвязка над internal/index: снимок vault и таска по
// API. Пакет не разбирает markdown и не знает про фронтматтер — за это
// отвечают internal/vault и internal/model.
package server

import (
	"net/http"

	"github.com/alkulagin-creator/tt/internal/index"
)

// Options — параметры сервера. Пока пусто: токен и адрес слушателя появятся
// в следующих задачах, когда до них дойдёт дело (`tt serve`).
type Options struct{}

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
	mux.HandleFunc("/api/task/{id}", s.handleTask)
	mux.HandleFunc("/", handleNotFound)
	return mux
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "маршрут не найден")
}
