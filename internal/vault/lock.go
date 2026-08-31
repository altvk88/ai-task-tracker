package vault

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrLocked — таску уже держит другой писатель.
var ErrLocked = errors.New("таска занята другим писателем")

// Сколько раз и с какой паузой повторять mkdir при ошибке, не означающей
// «замок занят». Суммарно около 50 мс — этого с запасом хватает, чтобы
// пережить удаление каталога другим процессом на Windows, и мало, чтобы
// заметно задержать команду при настоящей проблеме с правами.
const (
	lockAttempts   = 10
	lockRetryPause = 5 * time.Millisecond
)

// Lock берёт тот же замок, что и bash-агенты: атомарный mkdir
// <vault>/.locks/<ID>.lock. Совместимость важна — пока не все писатели
// переведены на tt, замок обязан пониматься обеими сторонами.
func Lock(vaultDir, id string) (unlock func(), err error) {
	dir := filepath.Join(vaultDir, ".locks", id+".lock")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}

	// Короткие повторы на ошибку, отличную от «уже существует». На Windows
	// os.Remove помечает каталог как удаляемый, и параллельный os.Mkdir по
	// тому же пути до завершения удаления возвращает ACCESS_DENIED, а не
	// ErrExist. Это временное состояние гонки двух писателей, а не отказ в
	// правах, и сдаваться на нём нельзя — иначе честное «занято» превращается
	// в невнятную ошибку. Настоящая проблема с правами переживёт все попытки
	// и будет возвращена как есть, а не замаскирована под ErrLocked.
	var lastErr error
	for attempt := 0; attempt < lockAttempts; attempt++ {
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return func() { os.Remove(dir) }, nil
		}
		if os.IsExist(err) {
			return nil, ErrLocked
		}
		lastErr = err
		time.Sleep(lockRetryPause)
	}
	return nil, lastErr
}
