package cli

import (
	"fmt"
	"io"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// Claim берёт таску в работу. Вся логика (проверка перехода, замок, блок
// claim) — в internal/taskop, чтобы CLI и HTTP-API брали таски одинаково.
func Claim(w io.Writer, vaultDir, id, agent string) error {
	res, err := taskop.Claim(vaultDir, id, agent)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: %s -> %s (агент %s)\n", id, res.From, res.To, agent)
	return nil
}

// Release отдаёт таску обратно в ready.
func Release(w io.Writer, vaultDir, id string) error {
	res, err := taskop.Release(vaultDir, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: %s -> %s, claim и замок сняты\n", id, res.From, res.To)
	return nil
}

// Reset расклинивает залипшую таску, снимая и чужой claim.
func Reset(w io.Writer, vaultDir, id string) error {
	res, err := taskop.Reset(vaultDir, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: %s -> %s, попытка %d\n", id, res.From, res.To, res.Task.Attempts)
	return nil
}
