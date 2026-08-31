package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/alkulagin-creator/tt/internal/taskop"
)

// Done закрывает таску и печатает результат вместе со списком тех, кого
// закрытие разблокировало. Молчать о чужом промоуте нельзя: человек должен
// видеть последствия своего действия, а не находить их постфактум в vault.
func Done(w io.Writer, vaultDir, id, result, agent string) error {
	res, err := taskop.Done(vaultDir, id, result, agent)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: %s -> %s\n", id, res.Result.From, res.Result.To)
	if len(res.Promoted) > 0 {
		fmt.Fprintf(w, "промоут в ready: %s\n", strings.Join(res.Promoted, ", "))
	}
	return nil
}
