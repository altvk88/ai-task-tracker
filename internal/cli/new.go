package cli

import (
	"fmt"
	"io"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// New создаёт таску и печатает результат. Правила (атомарная выдача ID,
// слаг, гейтинг, шаблон) живут в internal/taskop — том же месте, откуда их
// берёт Set, чтобы CLI не завёл вторую копию логики.
func New(w io.Writer, vaultDir string, opts taskop.NewOptions) error {
	res, err := taskop.New(vaultDir, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: %s\n", res.ID, res.Path)
	for _, warning := range res.Warnings {
		fmt.Fprintf(w, "предупреждение: %s\n", warning)
	}
	return nil
}
