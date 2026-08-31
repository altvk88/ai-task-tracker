package index

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounceDelay — пауза, за которую несколько событий на один путь
// схлопываются в одно применение. Редакторы и наш собственный writeAtomic
// пишут через временный файл и rename, поэтому одно логическое изменение
// всегда даёт несколько событий ОС подряд.
const debounceDelay = 50 * time.Millisecond

// Watch следит за <vaultDir>/tasks и точечно обновляет ix через Apply по
// изменившимся файлам — полное Snapshot-пересканирование на каждое событие
// не нужно. Блокирует вызывающего, пока не сработает ctx.Done() или
// fsnotify.Watcher не закроет свои каналы.
func Watch(ctx context.Context, ix *Index, vaultDir string) error {
	tasksDir := filepath.Join(vaultDir, "tasks")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := addDirRecursive(w, tasksDir); err != nil {
		return err
	}

	deb := newDebouncer(debounceDelay, func(path string) { ix.Apply(path) })
	defer deb.stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			handleEvent(w, ev, deb)
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				log.Printf("index: ошибка слежения за vault: %v", err)
			}
		}
	}
}

// handleEvent разбирает одно событие fsnotify: подписывается на новый
// каталог проекта (плюс подхватывает уже лежащие в нём таски) либо
// планирует применение изменившегося файла таски через дебаунсер.
func handleEvent(w *fsnotify.Watcher, ev fsnotify.Event, deb *debouncer) {
	name := filepath.Base(ev.Name)
	if isWriterTempFile(name) {
		return
	}

	if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
		if ev.Op&fsnotify.Create == 0 || strings.HasPrefix(name, "_") {
			return
		}
		if err := addDirRecursive(w, ev.Name); err != nil {
			log.Printf("index: не удалось подписаться на новый каталог %s: %v", ev.Name, err)
		}
		// Каталог мог появиться уже с файлами внутри (например, целиком
		// скопированный проект) — событий на сами файлы в этом случае не
		// будет, потому что в момент их создания мы ещё не подписались.
		scanExisting(ev.Name, deb)
		return
	}

	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		return
	}
	deb.trigger(ev.Name)
}

// addDirRecursive подписывает watcher на каталог и все его подкаталоги
// (кроме начинающихся с "_" — они, как и в vault.Scan, считаются образцами).
// fsnotify на Windows не рекурсивен, поэтому каждый каталог нужен отдельно.
func addDirRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && strings.HasPrefix(d.Name(), "_") {
			return fs.SkipDir
		}
		if err := w.Add(path); err != nil {
			log.Printf("index: не удалось подписаться на %s: %v", path, err)
		}
		return nil
	})
}

// scanExisting планирует применение всех .md-файлов, уже лежащих в только
// что подписанном каталоге.
func scanExisting(dir string, deb *debouncer) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			deb.trigger(path)
		}
		return nil
	})
}

// isWriterTempFile — временный файл нашего же атомарного писателя
// (vault.writeAtomic создаёт ".tt-*.tmp" в том же каталоге перед rename).
// Реагировать на него не нужно: интерес представляет только итоговый .md.
func isWriterTempFile(name string) bool {
	ok, _ := filepath.Match(".tt-*.tmp", name)
	return ok
}

// debouncer схлопывает несколько срабатываний на один путь в одно — с
// задержкой delay после последнего срабатывания.
type debouncer struct {
	delay time.Duration
	fn    func(path string)

	mu      sync.Mutex
	timers  map[string]*time.Timer
	stopped bool
}

func newDebouncer(delay time.Duration, fn func(string)) *debouncer {
	return &debouncer{delay: delay, fn: fn, timers: make(map[string]*time.Timer)}
}

// trigger переносит срабатывание на path ещё на delay вперёд. Если за это
// время придёт новое срабатывание — таймер переставится заново.
func (d *debouncer) trigger(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if t, ok := d.timers[path]; ok {
		t.Stop()
	}
	d.timers[path] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, path)
		stopped := d.stopped
		d.mu.Unlock()
		if !stopped {
			d.fn(path)
		}
	})
}

// stop отменяет все ожидающие таймеры. Уже выстрелившие (fn выполняется
// в этот момент в своей горутине) не прерывает — это быстрый вызов
// ix.Apply, который просто успевает доработать.
func (d *debouncer) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = nil
}
