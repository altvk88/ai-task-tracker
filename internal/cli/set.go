package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// writableFields — белый список ключей фронтматтера. Без него опечатка в имени
// ключа тихо добавила бы во фронтматтер мусорное поле.
var writableFields = map[string]bool{
	"status": true, "title": true, "priority": true, "due": true,
	"completed": true, "ready_at": true, "effort": true, "actual": true,
	"attempts": true, "spec": true, "result": true, "claim": true,
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
func Set(w io.Writer, vaultDir, id, key, value, agent string) error {
	if !writableFields[key] {
		return fmt.Errorf("поле %q не разрешено менять", key)
	}
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return err
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return err
	}
	byID := vault.ByID(tasks)
	task, ok := byID[id]
	if !ok {
		// Битая таска в индекс не попадает: её фронтматтер не разобрался, и ID
		// из него не извлечён. Ищем по тексту, чтобы не отвечать «не найдена»
		// на файл, который лежит на месте.
		if broken, found := brokenWithID(tasks, id); found {
			return fmt.Errorf("таска %s не разбирается (%s) — сначала почини фронтматтер", id, broken.ParseErr)
		}
		return fmt.Errorf("таска %s не найдена", id)
	}
	if task.ParseErr != "" {
		return fmt.Errorf("таска %s не разбирается (%s) — сначала почини фронтматтер", id, task.ParseErr)
	}

	if key != "status" {
		if err := vault.SetField(task.Path, key, value); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %s = %s\n", id, key, value)
		return nil
	}

	canon, known := schema.Normalize(value)
	if !known {
		return fmt.Errorf("неизвестный статус %q", value)
	}
	if err := model.CheckTransition(schema, task, canon, byID, agent); err != nil {
		return err
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
		default:
			return fmt.Errorf("%s: %w", id, err)
		}
	}

	if err := applyStatus(schema, task, canon); err != nil {
		if release != nil {
			release()
		}
		return err
	}
	if cur == "in-progress" && canon != "in-progress" {
		os.Remove(filepath.Join(vaultDir, ".locks", id+".lock"))
	}
	fmt.Fprintf(w, "%s: %s -> %s\n", id, task.Status, canon)
	return nil
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
