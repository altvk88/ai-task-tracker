package cli

import (
	"fmt"
	"io"
	"runtime"

	tt "github.com/alkulagin-creator/tt"
)

// PrintVersion печатает версию и то, что помогает разбирать жалобы: без
// платформы и версии Go непонятно, та ли это сборка, о которой идёт речь.
// Сам номер приходит из файла VERSION в корне репозитория — см. version.go там.
func PrintVersion(w io.Writer) {
	fmt.Fprintf(w, "tt %s (%s/%s, %s)\n", tt.Version(), runtime.GOOS, runtime.GOARCH, runtime.Version())
}
