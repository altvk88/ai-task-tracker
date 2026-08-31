package index

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// retryUntilEvent повторяет trigger, пока в ch не появится событие, либо не
// истечёт timeout. Нужен, чтобы пережить гонку между запуском горутины
// Watch (её настройка — Add() по каталогам — происходит асинхронно
// относительно теста) и первым действием теста: без ретрая первая запись
// может случиться раньше, чем watcher подписался на каталог, и повиснуть
// без единого события.
func retryUntilEvent(t *testing.T, ch <-chan Change, timeout time.Duration, trigger func()) Change {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		trigger()
		select {
		case c, ok := <-ch:
			if !ok {
				t.Fatal("канал подписчика закрылся неожиданно")
			}
			return c
		case <-time.After(150 * time.Millisecond):
		}
	}
	t.Fatal("событие так и не пришло")
	return Change{}
}

// awaitWatcherReady убеждается, что Watch успел подписаться на каталоги,
// прежде чем тест выполнит действие, которое повторить нельзя (удаление,
// проверку отсутствия события и т.п.). Пробный файл после себя убирает.
func awaitWatcherReady(t *testing.T, root string, ch <-chan Change) {
	t.Helper()
	probe := filepath.Join(root, "tasks", "alpha", "__probe.md")
	body := "---\nid: PRB-1\ntitle: probe\nstatus: ready\nproject: alpha\n---\n"
	retryUntilEvent(t, ch, 5*time.Second, func() {
		if err := os.WriteFile(probe, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	waitForChange(t, ch, 2*time.Second) // сливаем событие удаления пробника
}

func waitForChange(t *testing.T, ch <-chan Change, timeout time.Duration) Change {
	t.Helper()
	select {
	case c, ok := <-ch:
		if !ok {
			t.Fatal("канал подписчика закрылся неожиданно")
		}
		return c
	case <-time.After(timeout):
		t.Fatal("не дождались изменения")
	}
	return Change{}
}

// collectQuiet копит события из ch, пока не наступит пауза длиной quiet без
// новых событий, либо не истечёт max — общий потолок на случай, если поток
// событий не иссякает.
func collectQuiet(ch <-chan Change, quiet, max time.Duration) []Change {
	var events []Change
	deadline := time.Now().Add(max)
	for {
		select {
		case c := <-ch:
			events = append(events, c)
		case <-time.After(quiet):
			return events
		}
		if time.Now().After(deadline) {
			return events
		}
	}
}

func TestWatch_изменениеФайлаДоезжаетДоИндекса(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, unsubscribe := ix.Subscribe()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, ix, root)

	awaitWatcherReady(t, root, ch)

	path := filepath.Join(root, "tasks/alpha/one.md")
	updated := "---\nid: ALP-1\ntitle: первая правленая\nstatus: in-progress\nproject: alpha\n---\n"

	c := retryUntilEvent(t, ch, 3*time.Second, func() {
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if c.ID != "ALP-1" || c.Task.Status != "in-progress" {
		t.Errorf("неожиданное событие: %+v", c)
	}

	got, ok := ix.Get("ALP-1")
	if !ok || got.Status != "in-progress" {
		t.Errorf("индекс не обновился: %+v, %v", got, ok)
	}
}

func TestWatch_пачкаБыстрыхЗаписейДаётОдноПрименение(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, unsubscribe := ix.Subscribe()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, ix, root)

	awaitWatcherReady(t, root, ch)

	path := filepath.Join(root, "tasks/alpha/one.md")
	statuses := []string{"in-progress", "ready", "in-progress", "ready", "done"}
	for _, st := range statuses {
		body := "---\nid: ALP-1\ntitle: t\nstatus: " + st + "\nproject: alpha\n---\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	events := collectQuiet(ch, 300*time.Millisecond, 3*time.Second)
	if len(events) != 1 {
		t.Fatalf("после пяти быстрых записей ожидалось 1 событие, получено %d: %+v", len(events), events)
	}
	if events[0].Task.Status != "done" {
		t.Errorf("итоговый статус = %q, ожидался done", events[0].Task.Status)
	}

	got, ok := ix.Get("ALP-1")
	if !ok || got.Status != "done" {
		t.Errorf("индекс не отражает финальное состояние: %+v, %v", got, ok)
	}
}

func TestWatch_новыйКаталогПроектаПодхватывается(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, unsubscribe := ix.Subscribe()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, ix, root)

	awaitWatcherReady(t, root, ch)

	betaDir := filepath.Join(root, "tasks", "beta")
	betaFile := filepath.Join(betaDir, "one.md")
	betaMD := "---\nid: BETA-1\ntitle: новая\nstatus: ready\nproject: beta\n---\n"

	deadline := time.Now().Add(5 * time.Second)
	var found bool
	for time.Now().Before(deadline) && !found {
		if err := os.MkdirAll(betaDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(betaFile, []byte(betaMD), 0o644); err != nil {
			t.Fatal(err)
		}
		select {
		case c := <-ch:
			if c.ID == "BETA-1" {
				found = true
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !found {
		t.Fatal("новый каталог проекта не подхватился")
	}

	if _, ok := ix.Get("BETA-1"); !ok {
		t.Error("BETA-1 не появилась в индексе")
	}
}

func TestWatch_удалениеУбираетТаску(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, unsubscribe := ix.Subscribe()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, ix, root)

	awaitWatcherReady(t, root, ch)

	path := filepath.Join(root, "tasks/alpha/one.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	c := waitForChange(t, ch, 3*time.Second)
	if c.Kind != Removed || c.ID != "ALP-1" {
		t.Fatalf("неожиданное событие удаления: %+v", c)
	}
	if _, ok := ix.Get("ALP-1"); ok {
		t.Error("ALP-1 всё ещё в индексе после удаления файла")
	}
}

func TestWatch_временныйФайлПисателяИгнорируется(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, unsubscribe := ix.Subscribe()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, ix, root)

	awaitWatcherReady(t, root, ch)

	tmp := filepath.Join(root, "tasks/alpha/.tt-abc123.tmp")
	if err := os.WriteFile(tmp, []byte("что угодно"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp)

	events := collectQuiet(ch, 300*time.Millisecond, 1*time.Second)
	if len(events) != 0 {
		t.Fatalf("временный файл писателя не должен рождать события: %+v", events)
	}
}

func TestWatch_остановкаПоCancelНеОставляетГорутин(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, unsubscribe := ix.Subscribe()
	defer unsubscribe()

	before := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	go Watch(ctx, ix, root)

	awaitWatcherReady(t, root, ch)

	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for {
		runtime.Gosched()
		if runtime.NumGoroutine() <= before+2 { // небольшой допуск на планировщик/GC
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("похоже на утечку горутин: было %d, стало %d", before, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
