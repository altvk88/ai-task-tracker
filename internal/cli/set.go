package cli

import (
	"fmt"
	"io"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// Set меняет одно поле одной таски и печатает результат. Сами правила
// (белый список полей, нормализация статуса, проверка перехода, производные
// поля и замок) живут в internal/taskop — том же месте, откуда их берёт
// HTTP-API, чтобы CLI и веб не разошлись.
func Set(w io.Writer, vaultDir, id, key, value, agent string) error {
	res, err := taskop.Set(vaultDir, id, key, value, agent)
	if err != nil {
		return err
	}
	if key == "status" {
		fmt.Fprintf(w, "%s: %s -> %s\n", id, res.From, res.To)
	} else {
		fmt.Fprintf(w, "%s: %s = %s\n", id, key, value)
	}
	return nil
}
