package taskop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alkulagin-creator/tt/internal/vault"
)

// NewOptions — вход команды создания таски. Priority и Effort пустыми
// оставляют то, что уже стоит в шаблоне (для Priority это "medium").
type NewOptions struct {
	Project   string
	Title     string
	Priority  string
	Effort    string
	Spec      string
	DependsOn []string
}

// NewResult — что получилось: ID, путь к файлу, итоговый статус (ready или
// backlog по гейтингу) и предупреждения про зависимости, которых нет в vault
// (это не отказ — таска-блокер может появиться следом).
type NewResult struct {
	ID       string
	Path     string
	Status   string
	Warnings []string
}

// idLockDeadline — сколько ждать освобождения замка выдачи ID, прежде чем
// сдаться. Замок держится ровно на время одного New (создание файла внутри
// него небольшое), поэтому нескольких секунд с запасом хватает даже под
// конкурентной нагрузкой в тестах.
const idLockDeadline = 5 * time.Second

// projectCounter — часть фронтматтера projects/<project>.md, нужная для
// выдачи ID. Остальные поля файла проекта New не трогает.
type projectCounter struct {
	IDPrefix string `yaml:"id_prefix"`
	NextID   int    `yaml:"next_id"`
}

// New создаёт таску: атомарно выдаёт ID, подбирает свободное имя файла,
// заполняет шаблон и проставляет гейтинг статуса — ту же логику, что раньше
// делали руками /task и plan-to-tasks.
//
// Атомарность ID: инкремент next_id и вся запись файла таски идут под одним
// mkdir-замком <vault>/.locks/new_<project>.lock (тот же механизм, что
// vault.Lock использует для taskop.Set, — второй писатель, понимающий этот
// протокол, не нужен). Замок неблокирующий, поэтому New подряд пытается
// взять его с бэкоффом, а не проваливается на первом конфликте. Замок держит
// не только счётчик, но и весь остальной ход New: подбор имени файла — тоже
// чтение-затем-запись, и без общего замка два процесса могли бы выбрать один
// и тот же слаг.
func New(vaultDir string, opts NewOptions) (NewResult, error) {
	projPath := filepath.Join(vaultDir, "projects", opts.Project+".md")
	if _, err := os.Stat(projPath); err != nil {
		return NewResult{}, failf(KindNotFound, "проект %q не найден: нет файла %s", opts.Project, projPath)
	}
	templatePath := filepath.Join(vaultDir, "templates", "task-template.md")
	templateRaw, err := os.ReadFile(templatePath)
	if err != nil {
		return NewResult{}, failf(KindWrite, "не найден шаблон таски %s: %w", templatePath, err)
	}
	priority, err := normalizePriority(opts.Priority)
	if err != nil {
		return NewResult{}, failf(KindBadValue, "%w", err)
	}

	unlock, err := acquireIDLock(vaultDir, opts.Project)
	if err != nil {
		return NewResult{}, failf(KindWrite, "не удалось получить замок выдачи ID для %s: %w", opts.Project, err)
	}
	defer unlock()

	prefix, nextID, err := readProjectCounter(projPath)
	if err != nil {
		return NewResult{}, failf(KindUnparsable, "%w", err)
	}
	id := fmt.Sprintf("%s-%03d", prefix, nextID)
	if err := vault.SetField(projPath, "next_id", strconv.Itoa(nextID+1)); err != nil {
		return NewResult{}, failf(KindWrite, "%w", err)
	}

	taskDir := filepath.Join(vaultDir, "tasks", opts.Project)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return NewResult{}, failf(KindWrite, "%w", err)
	}
	slug, err := uniqueSlug(taskDir, slugify(opts.Title))
	if err != nil {
		return NewResult{}, failf(KindWrite, "%w", err)
	}
	taskPath := filepath.Join(taskDir, slug+".md")

	status, readyAt := gate(opts.DependsOn)
	today := time.Now().Format("2006-01-02")

	// Плейсхолдер даты в теле шаблона (строка "## Log") не входит во
	// фронтматтер и SetField его не видит — заменяем текстом, пока файл ещё
	// не записан. Плейсхолдер заголовка трогать не нужно: строка title
	// целиком перезаписывается SetField ниже.
	body := strings.ReplaceAll(string(templateRaw), `<% tp.date.now("YYYY-MM-DD") %>`, today)
	if err := os.WriteFile(taskPath, []byte(body), 0o644); err != nil {
		return NewResult{}, failf(KindWrite, "%w", err)
	}
	fields := [][2]string{
		{"id", id},
		{"title", opts.Title},
		{"status", status},
		{"project", opts.Project},
		{"priority", priority},
		{"created", today},
		{"ready_at", readyAt},
		{"effort", opts.Effort},
		{"spec", opts.Spec},
	}
	for _, f := range fields {
		if err := vault.SetField(taskPath, f[0], f[1]); err != nil {
			os.Remove(taskPath)
			return NewResult{}, failf(KindWrite, "%w", err)
		}
	}
	if err := vault.SetFieldRaw(taskPath, "blocked_by", dependsLiteral(opts.DependsOn)); err != nil {
		os.Remove(taskPath)
		return NewResult{}, failf(KindWrite, "%w", err)
	}

	return NewResult{ID: id, Path: taskPath, Status: status, Warnings: unknownDeps(vaultDir, opts.DependsOn)}, nil
}

// gate — гейтинг статуса, как в скилле plan-to-tasks: без зависимостей таска
// сразу ready с сегодняшней датой, с ними — backlog с пустым ready_at
// (поставит его авто-промоут, когда блокеры закроются).
func gate(dependsOn []string) (status, readyAt string) {
	if len(dependsOn) == 0 {
		return "ready", time.Now().Format("2006-01-02")
	}
	return "backlog", ""
}

func normalizePriority(p string) (string, error) {
	switch p {
	case "":
		return "medium", nil
	case "high", "medium", "low":
		return p, nil
	default:
		return "", fmt.Errorf("недопустимый приоритет %q: high|medium|low", p)
	}
}

// acquireIDLock берёт mkdir-замок под служебным именем, которое не может
// совпасть с настоящим ID таски (те всегда вида PREFIX-NNN). vault.Lock не
// блокируется сам — при занятом замке коротко ждём и пробуем снова.
func acquireIDLock(vaultDir, project string) (func(), error) {
	name := "new_" + project
	deadline := time.Now().Add(idLockDeadline)
	for {
		unlock, err := vault.Lock(vaultDir, name)
		if err == nil {
			return unlock, nil
		}
		if !errors.Is(err, vault.ErrLocked) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readProjectCounter(path string) (prefix string, next int, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	block, err := vault.Split(raw)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", path, err)
	}
	var pc projectCounter
	if err := yaml.Unmarshal(block, &pc); err != nil {
		return "", 0, fmt.Errorf("%s: %w", path, err)
	}
	if pc.IDPrefix == "" {
		return "", 0, fmt.Errorf("%s: нет id_prefix", path)
	}
	return pc.IDPrefix, pc.NextID, nil
}

// cyrillicToLatin — практическая транслитерация для слага файла. Заголовки
// в vault по факту переводятся человеком на латиницу (примеров кириллицы в
// именах файлов нет во всём vault), но у tt new нет переводчика — механическая
// транслитерация честнее, чем ронять команду или обрезать заголовок до пустоты.
var cyrillicToLatin = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "",
	'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

const maxSlugLen = 50

// slugify превращает заголовок в kebab-case имя файла: транслитерация
// кириллицы, всё, что не [a-z0-9], схлопывается в дефис, длина ограничена
// 50 символами.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		case cyrillicToLatin[r] != "" || r == 'ъ' || r == 'ь':
			b.WriteString(cyrillicToLatin[r])
		default:
			b.WriteByte('-')
		}
	}
	slug := strings.Join(strings.FieldsFunc(b.String(), func(r rune) bool { return r == '-' }), "-")
	if len(slug) > maxSlugLen {
		slug = strings.Trim(slug[:maxSlugLen], "-")
	}
	if slug == "" {
		slug = "task"
	}
	return slug
}

// uniqueSlug не даёт новой таске молча перезаписать существующий файл:
// коллизия имени — не повод отказать в создании таски (заголовки законно
// повторяются), поэтому подбирается различающий числовой суффикс.
func uniqueSlug(taskDir, base string) (string, error) {
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		if _, err := os.Stat(filepath.Join(taskDir, candidate+".md")); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("не удалось подобрать свободное имя файла для %q", base)
}

// dependsLiteral — буквальный YAML-список для blocked_by, пишется через
// SetFieldRaw, чтобы не получить квотированную строку вместо списка.
func dependsLiteral(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	return "[" + strings.Join(ids, ", ") + "]"
}

// unknownDeps — зависимости, которых нет в vault. Не отказ: таска-блокер
// может быть заведена следующим tt new в той же партии.
func unknownDeps(vaultDir string, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return nil
	}
	byID := vault.ByID(tasks)
	var missing []string
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			missing = append(missing, fmt.Sprintf("зависимость %s не найдена в vault", id))
		}
	}
	return missing
}
