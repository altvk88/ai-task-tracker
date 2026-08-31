package index

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// tempVault раскладывает минимальный vault и отдаёт его путь. Схему не
// кладём намеренно — New должен уметь работать на встроенной DefaultSchema.
func tempVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func aliceMD() string {
	return "---\nid: ALP-1\ntitle: первая\nstatus: ready\nproject: alpha\n---\n\nтело\n"
}

func bobMD() string {
	return "---\nid: ALP-2\ntitle: вторая\nstatus: done\nproject: alpha\n---\n"
}

func brokenMD() string {
	return "---\ntitle: без id и без закрывающего фенса\nstatus: ready\n"
}

func TestNew_строитСнимокСБитымиТасками(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md":    aliceMD(),
		"tasks/alpha/two.md":    bobMD(),
		"tasks/alpha/broken.md": brokenMD(),
	})

	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	snap := ix.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("в снимке %d тасок, ожидалось 3: %+v", len(snap), snap)
	}

	var brokenFound bool
	for _, task := range snap {
		if task.ParseErr != "" {
			brokenFound = true
			if task.Path == "" {
				t.Error("у битой таски обязан быть путь")
			}
		}
	}
	if !brokenFound {
		t.Error("битая таска потерялась при построении индекса")
	}

	if got, ok := ix.Get("ALP-1"); !ok || got.Title != "первая" {
		t.Errorf("Get(ALP-1) = %+v, %v", got, ok)
	}

	if ix.Schema() == nil {
		t.Error("Schema() вернул nil")
	}
}

func TestSnapshot_отдаётКопию(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	snap := ix.Snapshot()
	snap[0].Title = "испорчено"
	if snap[0].BlockedBy == nil {
		snap[0].BlockedBy = []string{"чужое"}
	}

	again, ok := ix.Get("ALP-1")
	if !ok {
		t.Fatal("ALP-1 не найдена")
	}
	if again.Title == "испорчено" {
		t.Error("правка снимка повлияла на внутреннее состояние индекса")
	}

	snap2 := ix.Snapshot()
	if snap2[0].Title == "испорчено" {
		t.Error("правка одного снимка повлияла на следующий")
	}
}

func TestApply_изменённыйФайлМеняетОднуЗапись(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
		"tasks/alpha/two.md": bobMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(root, "tasks/alpha/one.md")
	updated := "---\nid: ALP-1\ntitle: первая правленая\nstatus: in-progress\nproject: alpha\n---\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	change, changed := ix.Apply(path)
	if !changed {
		t.Fatal("Apply не заметил изменение файла")
	}
	if change.Kind != Updated {
		t.Errorf("Kind = %v, ожидался Updated", change.Kind)
	}
	if change.ID != "ALP-1" || change.Task.Status != "in-progress" {
		t.Errorf("неожиданный Change: %+v", change)
	}

	got, ok := ix.Get("ALP-1")
	if !ok || got.Status != "in-progress" {
		t.Errorf("Get(ALP-1) после Apply = %+v, %v", got, ok)
	}

	// Соседняя запись не тронута.
	other, ok := ix.Get("ALP-2")
	if !ok || other.Status != "done" {
		t.Errorf("ALP-2 задета посторонним Apply: %+v, %v", other, ok)
	}
}

func TestApply_безИзмененийВозвращаетFalse(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(root, "tasks/alpha/one.md")
	_, changed := ix.Apply(path)
	if changed {
		t.Error("Apply сообщил об изменении там, где файл не менялся")
	}
}

func TestApply_удалённыйФайлУбираетЗапись(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(root, "tasks/alpha/one.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	change, changed := ix.Apply(path)
	if !changed {
		t.Fatal("Apply не заметил удаление файла")
	}
	if change.Kind != Removed {
		t.Errorf("Kind = %v, ожидался Removed", change.Kind)
	}
	if change.ID != "ALP-1" {
		t.Errorf("Change.ID = %q, ожидался ALP-1", change.ID)
	}

	if _, ok := ix.Get("ALP-1"); ok {
		t.Error("ALP-1 всё ещё в индексе после удаления файла")
	}
	if len(ix.Snapshot()) != 0 {
		t.Error("снимок не пуст после удаления единственной таски")
	}

	// Повторный Apply по уже удалённому файлу — не событие.
	if _, changed := ix.Apply(path); changed {
		t.Error("повторный Apply по отсутствующему файлу снова сообщил об изменении")
	}
}

func TestApply_новыйФайлДобавляетЗапись(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(root, "tasks/alpha/new.md")
	if err := os.WriteFile(path, []byte(bobMD()), 0o644); err != nil {
		t.Fatal(err)
	}

	change, changed := ix.Apply(path)
	if !changed || change.Kind != Added {
		t.Fatalf("Apply на новый файл: change=%+v, changed=%v", change, changed)
	}
	if _, ok := ix.Get("ALP-2"); !ok {
		t.Error("новая таска не появилась в индексе")
	}
}

func TestSubscribe_получаетИзменениеИОтписываетсяБезПаники(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, unsubscribe := ix.Subscribe()

	path := filepath.Join(root, "tasks/alpha/one.md")
	updated := "---\nid: ALP-1\ntitle: первая\nstatus: in-progress\nproject: alpha\n---\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, changed := ix.Apply(path); !changed {
		t.Fatal("ожидалось изменение")
	}

	select {
	case c := <-ch:
		if c.ID != "ALP-1" || c.Kind != Updated {
			t.Errorf("неожиданное событие подписчика: %+v", c)
		}
	default:
		t.Fatal("подписчик не получил событие")
	}

	unsubscribe()
	unsubscribe() // идемпотентность: повторный вызов не должен паниковать

	// После отписки Apply не должен пытаться писать в закрытый канал.
	if err := os.WriteFile(path, []byte(aliceMD()), 0o644); err != nil {
		t.Fatal(err)
	}
	ix.Apply(path)
}

func TestSubscribe_переполнениеОтключаетПодписчикаЗакрытиемКанала(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, _ := ix.Subscribe()
	path := filepath.Join(root, "tasks/alpha/one.md")

	// Подписчик ничего не вычитывает — генерируем событий заведомо больше,
	// чем вмещает буфер (subscriberBuffer), чтобы вызвать переполнение.
	pair := [2]string{"in-progress", "ready"}
	for i := 0; i < subscriberBuffer+10; i++ {
		body := "---\nid: ALP-1\ntitle: t\nstatus: " + pair[i%2] + "\nproject: alpha\n---\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, changed := ix.Apply(path); !changed {
			t.Fatalf("шаг %d: изменение не замечено", i)
		}
	}

	// Канал обязан оказаться закрыт: тихой потери событий у живого
	// подписчика быть не должно, закрытие — однозначный сигнал "ты отстал".
	drainedClosed := false
	for {
		v, ok := <-ch
		if !ok {
			drainedClosed = true
			break
		}
		_ = v
	}
	if !drainedClosed {
		t.Error("канал подписчика не закрылся при переполнении буфера")
	}
}

func TestConcurrency_чтениеИApplyБезГонок(t *testing.T) {
	root := tempVault(t, map[string]string{
		"tasks/alpha/one.md": aliceMD(),
		"tasks/alpha/two.md": bobMD(),
	})
	ix, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	path := filepath.Join(root, "tasks/alpha/one.md")

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					snap := ix.Snapshot()
					for _, task := range snap {
						_ = task.ID
					}
					ix.Get("ALP-1")
				}
			}
		}()
	}

	sub, unsubscribe := ix.Subscribe()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case _, ok := <-sub:
				if !ok {
					return
				}
			}
		}
	}()

	for i := 0; i < 50; i++ {
		st := "ready"
		if i%2 == 0 {
			st = "in-progress"
		}
		body := "---\nid: ALP-1\ntitle: t\nstatus: " + st + "\nproject: alpha\n---\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		ix.Apply(path)
	}

	close(stop)
	unsubscribe()
	wg.Wait()
}
