package vault

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLockIsExclusive(t *testing.T) {
	root := t.TempDir()

	unlock, err := Lock(root, "WEB-150")
	if err != nil {
		t.Fatalf("первый захват не удался: %v", err)
	}
	if _, err := Lock(root, "WEB-150"); !errors.Is(err, ErrLocked) {
		t.Fatalf("второй захват дал %v, ожидалась ErrLocked", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".locks", "WEB-150.lock")); err != nil {
		t.Errorf("каталог замка не создан: %v", err)
	}

	unlock()
	unlock2, err := Lock(root, "WEB-150")
	if err != nil {
		t.Fatalf("после освобождения захват не удался: %v", err)
	}
	unlock2()
	if _, err := os.Stat(filepath.Join(root, ".locks", "WEB-150.lock")); !os.IsNotExist(err) {
		t.Error("каталог замка не удалён после освобождения")
	}
}

func TestLockDifferentTasksDoNotCollide(t *testing.T) {
	root := t.TempDir()
	u1, err := Lock(root, "WEB-1")
	if err != nil {
		t.Fatal(err)
	}
	u2, err := Lock(root, "WEB-2")
	if err != nil {
		t.Fatalf("замок на другую таску не должен конфликтовать: %v", err)
	}
	u1()
	u2()
}

func TestLockRace(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := Lock(root, "WEB-150")
			if err != nil {
				return
			}
			mu.Lock()
			won++
			mu.Unlock()
			unlock()
		}()
	}
	wg.Wait()
	if won == 0 {
		t.Fatal("замок не взял никто")
	}
	t.Logf("захватов из 50 попыток: %d", won)
}
