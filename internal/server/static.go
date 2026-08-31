package server

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/alkulagin-creator/tt/web"
)

// staticHandler отдаёт web/dist (вшит в бинарник package web — см. web/embed.go;
// go:embed не умеет ссылаться на родительский каталог, поэтому директива живёт
// там, а не здесь) на любом пути, кроме /api/*. У клиента пока нет
// собственного роутера (это заглушка перед полноценной доской), поэтому
// неизвестный путь тоже получает index.html — этого достаточно для корня.
//
// На чистом клоне без собранного фронта (npm run build не запускался) dist
// содержит только .gitkeep и index.html-заглушку — Handler() отдаёт её вместо
// паники сборки.
func staticHandler() http.Handler {
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// Каталог всегда встроен по абсолютному пути выше — эта ошибка
		// означает баг в самом коде, а не в окружении пользователя.
		panic(err)
	}
	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(sub, fsPath(r.URL.Path)); err != nil {
			// Не подменяем r.URL.Path на "/index.html" и не отдаём это через
			// fileServer: у http.FileServer есть встроенная канонизация,
			// которая при виде имени "index.html" в пути шлёт 301 на "./" —
			// для SPA-фолбэка это не redirect, а сама страница.
			serveIndex(w, sub)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// notBuiltPage показывается, когда в бинарник вшит пустой dist: фронт не
// собирали перед `go build`. Страница живёт в коде, а не файлом в dist,
// потому что любой отслеживаемый файл там перезаписывается сборкой и пачкает
// рабочее дерево, а закоммиченный собранный index.html ссылался бы на
// гитигнорируемый JS — чистый клон получил бы битую страницу.
const notBuiltPage = `<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><title>tt</title></head>
<body style="font-family:system-ui;max-width:40em;margin:3em auto;line-height:1.5">
<h1>Фронтенд не собран</h1>
<p>Бинарник собран без веб-бандла. Соберите фронт и пересоберите tt:</p>
<pre>cd web &amp;&amp; npm install &amp;&amp; npm run build &amp;&amp; cd ..
go build -o tt.exe ./cmd/tt</pre>
<p>API при этом работает: <code>/api/snapshot</code>, <code>/api/events</code>.</p>
</body></html>`

// serveIndex отдаёт содержимое index.html напрямую, в обход http.FileServer.
func serveIndex(w http.ResponseWriter, sub fs.FS) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		_, _ = io.WriteString(w, notBuiltPage)
		return
	}
	_, _ = w.Write(data)
}

// fsPath переводит URL-путь в путь внутри fs.FS: без ведущего "/", "" для
// корня заменяется на "index.html" (так требует io/fs.Stat).
func fsPath(urlPath string) string {
	p := urlPath
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	if p == "" {
		p = "index.html"
	}
	return p
}
