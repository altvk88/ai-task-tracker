package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// tokenBytes — длина токена в байтах до hex-кодирования. 32 байта — запас
// сверх, скажем, 128 бит энтропии, которых достаточно, чтобы токен нельзя
// было перебрать по сети.
const tokenBytes = 32

// GenerateToken генерирует криптостойкий токен для доступа на запись.
// Печатается один раз при старте tt serve, если --token не задан явно.
func GenerateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// authMiddleware пускает без токена: (1) любой запрос с loopback-адреса —
// у локального пользователя и так есть доступ к файлам vault напрямую;
// (2) GET/HEAD с любого адреса — иначе открыть доску с телефона по ссылке
// было бы мучением. Всё остальное с не-loopback адреса требует верного
// токена.
//
// Пустой Options.Token НЕ отключает проверку, а запрещает удалённую запись
// вовсе: сервер без настроенного токена никого аутентифицировать не может,
// и правильный ответ на попытку — отказ, а не пропуск. Иначе вторая точка
// входа, забывшая передать токен, тихо открыла бы запись всей локальной сети.
// На loopback это ничего не меняет: там запись разрешена и без токена.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLoopbackAddr(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if s.opts.Token == "" {
			writeError(w, http.StatusUnauthorized, "удалённая запись запрещена: сервер запущен без токена")
			return
		}
		if !tokenValid(s.opts.Token, requestToken(r)) {
			writeError(w, http.StatusUnauthorized, "нужен токен на запись: заголовок Authorization: Bearer <токен> или ?token=")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackAddr разбирает r.RemoteAddr ("host:port") и проверяет host через
// net.IP.IsLoopback — это покрывает и "::1", и весь диапазон 127.0.0.0/8, в
// отличие от сравнения со строкой "127.0.0.1". RemoteAddr выставляет сам
// net/http по факту TCP-соединения, подделать его с удалённой машины нельзя
// (в отличие от заголовков вроде X-Forwarded-For, которым мы поэтому и не
// доверяем — прокси перед нами нет).
func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// requestToken достаёт токен из заголовка Authorization: Bearer <токен> или,
// если заголовка нет, из параметра ?token= — второе нужно, чтобы ссылку на
// доску можно было открыть с телефона одним касанием.
func requestToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if tok, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return tok
		}
		return ""
	}
	return r.URL.Query().Get("token")
}

// tokenValid сравнивает токены постоянным по времени, чтобы не утекать их
// совпадение через длительность сравнения.
func tokenValid(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
