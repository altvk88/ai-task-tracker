// Package web вшивает собранный Svelte-бандл (dist/) в бинарник tt.
//
// go:embed не умеет ссылаться на родительский каталог ("../"), поэтому
// директива живёт здесь же, рядом с dist, а не в internal/server — оттуда
// каталог только импортируется.
package web

import "embed"

// Dist — содержимое web/dist целиком, включая index.html и assets/.
//
// go:embed падает на пустом или отсутствующем каталоге, поэтому в git всегда
// лежит хотя бы dist/.gitkeep и dist/index.html-заглушка (см. корневой
// .gitignore — остальное содержимое dist игнорируется и появляется только
// после `npm run build` в этом каталоге).
//
//go:embed all:dist
var Dist embed.FS
