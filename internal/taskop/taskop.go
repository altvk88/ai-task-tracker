// Package taskop — запись в таску: единственное место, где живут правила
// смены статуса (белый список полей, нормализация, проверка перехода,
// производные completed/ready_at/claim и замок).
//
// Пакет заведён ради одного: и CLI (`tt set`), и HTTP-API должны менять
// статус ровно одинаково. Вторая копия правил в сервере разошлась бы с
// первой правкой — ровно та беда, из-за которой раньше спорили доска и
// фронтматтер.
//
// Зависимости идут только вниз: taskop -> vault, model. Про транспорт
// (io.Writer, http) пакет не знает.
package taskop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// Kind — вид отказа. Нужен вызывающему коду, чтобы принять своё решение по
// одной и той же ошибке: CLI печатает текст, HTTP-API выбирает код ответа.
type Kind int

const (
	KindNotFound   Kind = iota // таски с таким ID в vault нет
	KindUnparsable             // файл на месте, но фронтматтер не разбирается
	KindBadValue               // недопустимое поле или неизвестный статус
	KindRejected               // переход запрещён правилами или таску держит другой писатель
	KindWrite                  // не удалось прочитать или записать файл
)

// Error — отказ операции записи с указанием вида.
type Error struct {
	Kind Kind
	msg  string
	err  error
}

func (e *Error) Error() string { return e.msg }
func (e *Error) Unwrap() error { return e.err }

func failf(kind Kind, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	return &Error{Kind: kind, msg: err.Error(), err: errors.Unwrap(err)}
}

// KindOf достаёт вид отказа. Для ошибки не из этого пакета — false.
func KindOf(err error) (Kind, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind, true
	}
	return 0, false
}

// writableFields — белый список ключей фронтматтера. Без него опечатка в имени
// ключа тихо добавила бы во фронтматтер мусорное поле.
var writableFields = map[string]bool{
	"status": true, "title": true, "priority": true, "due": true,
	"completed": true, "ready_at": true, "effort": true, "actual": true,
	"attempts": true, "spec": true, "result": true, "claim": true,
}

// Result — что получилось. Task перечитана с диска после записи, поэтому её
// можно отдавать клиенту как актуальное состояние.
type Result struct {
	Task model.Task
	From string // статус до правки, как он лежал в файле
	To   string // канонический статус после правки; пусто, если менялся не статус
}

// SchemaPath — путь к общему контракту правил внутри vault.
func SchemaPath(vaultDir string) string {
	return filepath.Join(vaultDir, ".task-tracker", "schema.json")
}

// Set меняет одно поле одной таски. Смена статуса дополнительно проставляет
// completed/ready_at и снимает claim с замком по правилам схемы — ровно то,
// что раньше делали вручную /done и обратный синк доски.
//
// Замок <vault>/.locks/<ID>.lock здесь не мьютекс на время команды, а признак
// «таска в работе»: его ставят и снимают bash-агенты (lib/tt-lock.sh), и живёт
// он всё время, пока таска в in-progress. Поэтому берём его при взятии в работу
// и снимаем при уходе из неё, а не вокруг каждой записи — иначе правка любого
// поля у своей же таски в работе упиралась бы в собственный замок.
func Set(vaultDir, id, key, value, agent string) (Result, error) {
	if !writableFields[key] {
		return Result{}, failf(KindBadValue, "поле %q не разрешено менять", key)
	}
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	byID := vault.ByID(tasks)
	task, ok := byID[id]
	if !ok {
		// Битая таска в индекс не попадает: её фронтматтер не разобрался, и ID
		// из него не извлечён. Ищем по тексту, чтобы не отвечать «не найдена»
		// на файл, который лежит на месте.
		if broken, found := brokenWithID(tasks, id); found {
			return Result{}, unparsable(id, broken.ParseErr)
		}
		return Result{}, failf(KindNotFound, "таска %s не найдена", id)
	}
	if task.ParseErr != "" {
		return Result{}, unparsable(id, task.ParseErr)
	}

	if key != "status" {
		if err := vault.SetField(task.Path, key, value); err != nil {
			return Result{}, failf(KindWrite, "%w", err)
		}
		return reread(task, task.Status, "")
	}

	canon, known := schema.Normalize(value)
	if !known {
		return Result{}, failf(KindBadValue, "неизвестный статус %q", value)
	}
	if err := model.CheckTransition(schema, task, canon, byID, agent); err != nil {
		return Result{}, failf(KindRejected, "%w", err)
	}
	cur, _ := schema.Normalize(task.Status)

	// Замок берём только на взятии в работу и только если он свободен: чужой
	// не срываем. Свой же замок, оставшийся с прошлого взятия, помехой не считаем.
	var release func()
	if canon == "in-progress" {
		unlock, err := vault.Lock(vaultDir, id)
		switch {
		case err == nil:
			release = unlock
		case errors.Is(err, vault.ErrLocked) && cur == "in-progress":
			// Таска уже в работе, и замок парный её статусу — это не чужой
			// захват. Чужого владельца отсёк бы CheckTransition выше по claim.
		case errors.Is(err, vault.ErrLocked):
			return Result{}, failf(KindRejected, "%s: %w", id, err)
		default:
			return Result{}, failf(KindWrite, "%s: %w", id, err)
		}
	}

	if err := applyStatus(schema, task, canon); err != nil {
		if release != nil {
			release()
		}
		return Result{}, failf(KindWrite, "%w", err)
	}
	if cur == "in-progress" && canon != "in-progress" {
		os.Remove(filepath.Join(vaultDir, ".locks", id+".lock"))
	}
	return reread(task, task.Status, canon)
}

func unparsable(id, parseErr string) *Error {
	return failf(KindUnparsable, "таска %s не разбирается (%s) — сначала почини фронтматтер", id, parseErr)
}

// reread перечитывает таску после записи: вызывающему нужно актуальное
// состояние, а не то, что лежало в памяти до правки.
func reread(task model.Task, from, to string) (Result, error) {
	raw, err := os.ReadFile(task.Path)
	if err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	fresh, parseErr := vault.Parse(raw)
	fresh.Path = task.Path
	if parseErr != nil {
		fresh.ParseErr = parseErr.Error()
	}
	return Result{Task: fresh, From: from, To: to}, nil
}

// applyStatus пишет статус и производные от него поля. Уже заполненные
// completed/ready_at не перебиваем: первая дата достовернее последней.
func applyStatus(schema *model.Schema, task model.Task, canon string) error {
	today := time.Now().Format("2006-01-02")
	if err := vault.SetField(task.Path, "status", canon); err != nil {
		return err
	}
	if schema.SetsCompletedOn(canon) && task.Completed == "" {
		if err := vault.SetField(task.Path, "completed", today); err != nil {
			return err
		}
	}
	if schema.SetsReadyAtOn(canon) && task.ReadyAt == "" {
		if err := vault.SetField(task.Path, "ready_at", today); err != nil {
			return err
		}
	}
	if schema.ClearsClaimOn(canon) && task.Claimed() {
		if err := vault.SetField(task.Path, "claim", ""); err != nil {
			return err
		}
	}
	return nil
}

// brokenWithID ищет ID среди тасок, чей фронтматтер не разобрался.
func brokenWithID(tasks []model.Task, id string) (model.Task, bool) {
	for _, t := range tasks {
		if t.ParseErr == "" {
			continue
		}
		raw, err := os.ReadFile(t.Path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if rest, found := strings.CutPrefix(line, "id:"); found && strings.TrimSpace(rest) == id {
				return t, true
			}
		}
	}
	return model.Task{}, false
}
