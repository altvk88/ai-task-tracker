package vault

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrLocked — таску уже держит другой писатель.
var ErrLocked = errors.New("таска занята другим писателем")

// Lock берёт тот же замок, что и bash-агенты: атомарный mkdir
// <vault>/.locks/<ID>.lock. Совместимость важна — пока не все писатели
// переведены на tt, замок обязан пониматься обеими сторонами.
func Lock(vaultDir, id string) (unlock func(), err error) {
	dir := filepath.Join(vaultDir, ".locks", id+".lock")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		if os.IsExist(err) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return func() { os.Remove(dir) }, nil
}
