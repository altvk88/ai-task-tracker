package taskop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// Claim берёт таску в работу от имени agent: проверяет переход, ставит замок,
// пишет блок claim и статус in-progress. До этой команды `tt set X status
// in-progress` замок ставил, но владельца в файле не фиксировал — и guard по
// чужому claim на таски, взятые через tt, не срабатывал вовсе.
//
// Повторный claim своим же агентом законен и ничего не ломает: блок
// перезаписывается теми же значениями, замок остаётся стоять.
func Claim(vaultDir, id, agent string) (Result, error) {
	schema, byID, task, err := locate(vaultDir, id)
	if err != nil {
		return Result{}, err
	}
	if err := model.CheckTransition(schema, task, "in-progress", byID, agent); err != nil {
		return Result{}, failf(KindRejected, "%w", err)
	}
	cur, _ := schema.Normalize(task.Status)

	// Замок здесь — долгоживущий признак «таска в работе», а не мьютекс на
	// время команды: снимать его по выходу нельзя, иначе защиты от
	// параллельного захвата не останется. Свой же замок у таски, которая уже
	// in-progress, чужим захватом не считается — чужого владельца отсёк бы
	// CheckTransition выше по claim.
	var release func()
	unlock, err := vault.Lock(vaultDir, id)
	switch {
	case err == nil:
		release = unlock
	case errors.Is(err, vault.ErrLocked) && cur == "in-progress":
	case errors.Is(err, vault.ErrLocked):
		return Result{}, failf(KindRejected, "%s: %w", id, err)
	default:
		return Result{}, failf(KindWrite, "%s: %w", id, err)
	}

	fields := [][2]string{
		{"agent", agent},
		{"host", hostname()},
		{"branch", projectBranch(vaultDir, task.Project)},
		{"started", time.Now().Format("2006-01-02")},
	}
	if err := vault.SetBlock(task.Path, "claim", fields); err != nil {
		if release != nil {
			release()
		}
		return Result{}, failf(KindWrite, "%w", err)
	}
	if err := vault.SetField(task.Path, "status", "in-progress"); err != nil {
		if release != nil {
			release()
		}
		return Result{}, failf(KindWrite, "%w", err)
	}
	return reread(task, task.Status, "in-progress")
}

// Release отдаёт таску обратно: снимает claim с замком и возвращает статус
// ready. Обратная операция к Claim для случая «взял, но не делаю».
func Release(vaultDir, id string) (Result, error) {
	schema, _, task, err := locate(vaultDir, id)
	if err != nil {
		return Result{}, err
	}
	if err := applyStatus(schema, task, "ready"); err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	unlockDir(vaultDir, id)
	return reread(task, task.Status, "ready")
}

// Reset принудительно расклинивает таску: снимает замок и claim, даже чужие,
// возвращает статус ready и засчитывает попытку. Замена /work --reset.
//
// Чужой claim здесь не препятствие, а причина: команда существует ровно для
// упавшей сессии, которая оставила таску в in-progress со своим замком.
// Авто-снятия по таймауту нет намеренно — решение принимает человек.
func Reset(vaultDir, id string) (Result, error) {
	schema, _, task, err := locate(vaultDir, id)
	if err != nil {
		return Result{}, err
	}
	if err := applyStatus(schema, task, "ready"); err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	// applyStatus снимает только распознанный claim; здесь чистим ключ в
	// любом случае — на залипшей таске он бывает и скалярным, и мусорным.
	if err := vault.SetField(task.Path, "claim", ""); err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	if err := vault.SetField(task.Path, "attempts", strconv.Itoa(task.Attempts+1)); err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	unlockDir(vaultDir, id)
	return reread(task, task.Status, "ready")
}

// unlockDir снимает замок таски. Отсутствие замка ошибкой не считается:
// на залипшей таске его могли снять руками.
func unlockDir(vaultDir, id string) {
	os.Remove(filepath.Join(vaultDir, ".locks", id+".lock"))
}

// locate читает схему и находит таску по ID, отличая «нет такой» от «файл
// есть, но не разбирается»: битая таска в индекс не попадает, и отвечать на
// неё «не найдена» было бы неправдой.
func locate(vaultDir, id string) (*model.Schema, map[string]model.Task, model.Task, error) {
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return nil, nil, model.Task{}, failf(KindWrite, "%w", err)
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return nil, nil, model.Task{}, failf(KindWrite, "%w", err)
	}
	byID := vault.ByID(tasks)
	task, ok := byID[id]
	if !ok {
		if broken, found := brokenWithID(tasks, id); found {
			return nil, nil, model.Task{}, unparsable(id, broken.ParseErr)
		}
		return nil, nil, model.Task{}, failf(KindNotFound, "таска %s не найдена", id)
	}
	if task.ParseErr != "" {
		return nil, nil, model.Task{}, unparsable(id, task.ParseErr)
	}
	return schema, byID, task, nil
}

// hostname — имя машины для блока claim. Ошибку не поднимаем: пустой host
// хуже выдуманного только тем, что менее информативен.
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// projectBranch — текущая ветка репозитория проекта. Путь берётся из поля
// repo в projects/<project>.md. Репозитория нет, поле пустое, git недоступен
// или каталог не репозиторий — поле branch остаётся пустым: выдумывать
// «main» нельзя, это была бы ложь в файле таски.
func projectBranch(vaultDir, project string) string {
	if project == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(vaultDir, "projects", project+".md"))
	if err != nil {
		return ""
	}
	block, err := vault.Split(raw)
	if err != nil {
		return ""
	}
	var meta struct {
		Repo string `yaml:"repo"`
	}
	if err := yaml.Unmarshal(block, &meta); err != nil || meta.Repo == "" {
		return ""
	}
	// branch --show-current, а не rev-parse --abbrev-ref HEAD: первый честно
	// отдаёт пустую строку на отделённой HEAD и работает в репозитории без
	// коммитов, где второй падает с ошибкой.
	out, err := exec.Command("git", "-C", meta.Repo, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
