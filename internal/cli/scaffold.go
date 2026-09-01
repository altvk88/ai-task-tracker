package cli

import (
	"fmt"
	"io"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// Scaffold создаёт структуру vault и печатает, что именно сделано — отдельно
// созданное и то, что уже было и осталось нетронутым.
func Scaffold(w io.Writer, vaultDir string, opts taskop.ScaffoldOptions) error {
	res, err := taskop.Scaffold(vaultDir, opts)
	if err != nil {
		return err
	}
	for _, c := range res.Created {
		fmt.Fprintf(w, "создано: %s\n", c)
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(w, "уже было: %s\n", s)
	}
	if len(res.Created) == 0 {
		fmt.Fprintln(w, "структура vault уже полная, ничего не создано")
	}
	return nil
}
