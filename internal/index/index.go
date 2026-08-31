// Package index — снимок vault в памяти. Строится один раз через vault.Scan,
// дальше живёт на потоке точечных Apply(path) по изменившимся файлам —
// полный пересканирование 1277 тасок не нужно на каждое изменение.
//
// Пакет ничего не знает про транспорт: рассылает изменения подписчикам через
// канал, а кто на другом конце (HTTP-сервер, тест, CLI) — не его забота.
// Импорт net/http здесь недопустим.
package index

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// ChangeKind — вид изменения, о котором узнаёт подписчик.
type ChangeKind int

const (
	Added ChangeKind = iota
	Updated
	Removed
)

// Change — описание одного изменения для подписчиков.
type Change struct {
	ID   string     // ID таски; пусто, если файл не разобрался и ID неизвестен
	Path string     // путь к файлу — всегда заполнен
	Kind ChangeKind // Added | Updated | Removed
	Task model.Task // текущее состояние; для Removed — последнее известное
}

// subscriberBuffer — сколько событий подписчик может накопить, не вычитывая.
// При переполнении подписчик отключается (см. broadcast) — тихой потери
// событий у живого клиента быть не должно.
const subscriberBuffer = 64

// Index — снимок vault в памяти: таски по пути и по ID, плюс схема флоу.
type Index struct {
	mu     sync.RWMutex
	schema *model.Schema
	byPath map[string]model.Task
	byID   map[string]model.Task

	subMu   sync.Mutex
	subs    map[int]chan Change
	nextSub int
}

// schemaPath — путь к общему контракту правил внутри vault. Продублирован из
// cli.SchemaPath, а не импортирован: cli — внешний слой (командная строка),
// и когда там появится команда `tt serve`, использующая index, импорт
// index -> cli -> index замкнётся в цикл. Путь — одна строка, дублировать
// дешевле, чем ломать направление зависимостей.
func schemaPath(vaultDir string) string {
	return filepath.Join(vaultDir, ".task-tracker", "schema.json")
}

// New строит снимок vault: полный обход через vault.Scan плюс схема флоу.
func New(vaultDir string) (*Index, error) {
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return nil, err
	}
	schema, err := model.LoadSchema(schemaPath(vaultDir))
	if err != nil {
		return nil, err
	}

	ix := &Index{
		schema: schema,
		byPath: make(map[string]model.Task, len(tasks)),
		byID:   make(map[string]model.Task, len(tasks)),
		subs:   make(map[int]chan Change),
	}
	for _, t := range tasks {
		ix.byPath[t.Path] = t
		if t.ID != "" {
			ix.byID[t.ID] = t
		}
	}
	return ix, nil
}

// Schema отдаёт схему флоу. Схема не меняется после New, поэтому блокировка
// не нужна.
func (ix *Index) Schema() *model.Schema {
	return ix.schema
}

// Snapshot отдаёт копию всех тасок, отсортированную по пути. Копия — чтобы
// правка результата вызывающим кодом не портила внутреннее состояние
// индекса (Task содержит срезы и указатель Claim, они клонируются).
func (ix *Index) Snapshot() []model.Task {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	out := make([]model.Task, 0, len(ix.byPath))
	for _, t := range ix.byPath {
		out = append(out, cloneTask(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Get отдаёт таску по ID (копию, по тем же причинам, что и Snapshot).
func (ix *Index) Get(id string) (model.Task, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	t, ok := ix.byID[id]
	if !ok {
		return model.Task{}, false
	}
	return cloneTask(t), true
}

// Apply перечитывает один файл и обновляет индекс. Второй возврат — было ли
// изменение вообще: если содержимое не поменялось, рассылать нечего.
func (ix *Index) Apply(path string) (Change, bool) {
	ix.mu.Lock()

	old, existed := ix.byPath[path]

	info, statErr := os.Stat(path)
	if statErr != nil || info.IsDir() {
		if !existed {
			ix.mu.Unlock()
			return Change{}, false
		}
		delete(ix.byPath, path)
		if old.ID != "" {
			delete(ix.byID, old.ID)
		}
		change := Change{ID: old.ID, Path: path, Kind: Removed, Task: old}
		ix.mu.Unlock()
		ix.broadcast(change)
		return change, true
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		// Файл был по Stat, но прочитать не вышло (гонка с параллельным
		// удалением) — трактуем как отсутствие изменений, старую запись
		// не теряем: следующий Apply на этом же пути разберётся заново.
		ix.mu.Unlock()
		return Change{}, false
	}

	task, parseErr := vault.Parse(raw)
	task.Path = path
	if parseErr != nil {
		task.ParseErr = parseErr.Error()
	}

	if existed && reflect.DeepEqual(old, task) {
		ix.mu.Unlock()
		return Change{}, false
	}

	ix.byPath[path] = task
	if existed && old.ID != "" && old.ID != task.ID {
		delete(ix.byID, old.ID)
	}
	if task.ID != "" {
		ix.byID[task.ID] = task
	}

	kind := Added
	if existed {
		kind = Updated
	}
	change := Change{ID: task.ID, Path: path, Kind: kind, Task: task}
	ix.mu.Unlock()
	ix.broadcast(change)
	return change, true
}

// Subscribe отдаёт канал изменений и функцию отписки. Отписка идемпотентна.
//
// Канал буферизован (subscriberBuffer); при переполнении подписчик не
// блокирует Apply — событие не встаёт в очередь, а сам подписчик
// отключается: канал закрывается и удаляется из списка. Закрытие — это
// однозначный, невозможный не заметить сигнал "ты отстал, пересобери
// состояние через Snapshot() и подпишись заново", в отличие от молчаливого
// отбрасывания отдельных событий, которое живой клиент не увидит вовсе.
func (ix *Index) Subscribe() (<-chan Change, func()) {
	ch := make(chan Change, subscriberBuffer)

	ix.subMu.Lock()
	id := ix.nextSub
	ix.nextSub++
	ix.subs[id] = ch
	ix.subMu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			ix.subMu.Lock()
			if _, ok := ix.subs[id]; ok {
				delete(ix.subs, id)
				close(ch)
			}
			ix.subMu.Unlock()
		})
	}
	return ch, unsubscribe
}

// broadcast рассылает изменение подписчикам, не блокируясь на медленных:
// при полном буфере подписчик отключается (см. Subscribe).
func (ix *Index) broadcast(c Change) {
	ix.subMu.Lock()
	defer ix.subMu.Unlock()
	for id, ch := range ix.subs {
		select {
		case ch <- c:
		default:
			close(ch)
			delete(ix.subs, id)
		}
	}
}

// cloneTask копирует таску так, чтобы правка среза или Claim в результате
// не задевала данные, лежащие в индексе.
func cloneTask(t model.Task) model.Task {
	if t.BlockedBy != nil {
		t.BlockedBy = append([]string(nil), t.BlockedBy...)
	}
	if t.Verify != nil {
		t.Verify = append(model.StringList(nil), t.Verify...)
	}
	if t.Claim != nil {
		c := *t.Claim
		t.Claim = &c
	}
	return t
}
