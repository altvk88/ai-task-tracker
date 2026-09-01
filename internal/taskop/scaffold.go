package taskop

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alkulagin-creator/tt/internal/model"
)

//go:embed scaffold_template.md
var scaffoldTemplateFS embed.FS

// ScaffoldOptions — вход команды создания структуры vault. Пустые поля
// заполняются разумными значениями по умолчанию — установщику не нужно
// спрашивать про них отдельно.
type ScaffoldOptions struct {
	Project  string // имя первого проекта; пусто — берётся из имени каталога vault
	IDPrefix string // префикс ID проекта; пусто — выводится из имени проекта
}

// ScaffoldResult — что именно сделано: список созданного и список того, что
// уже было и осталось нетронутым. Оба в фиксированном порядке — воспроизводимо
// между запусками.
type ScaffoldResult struct {
	Created []string
	Skipped []string
}

// Scaffold создаёт минимальный рабочий каркас vault — ровно то, чего требует
// код tt (см. README и TT-048), а не всё, что исторически лежит в vault для
// Obsidian: tasks/, шаблон таски, схему флоу и один проект со счётчиком ID.
//
// Существующие файлы не перезаписываются: чего нет — создаётся, что есть —
// остаётся как было. Это делает команду безопасной на непустом каталоге —
// повторный запуск на готовом vault ничего не портит.
func Scaffold(vaultDir string, opts ScaffoldOptions) (ScaffoldResult, error) {
	var res ScaffoldResult
	note := func(created bool, rel string) {
		if created {
			res.Created = append(res.Created, rel)
		} else {
			res.Skipped = append(res.Skipped, rel)
		}
	}

	if err := os.MkdirAll(vaultDir, 0o755); err != nil {
		return res, fmt.Errorf("не удалось создать каталог vault %s: %w", vaultDir, err)
	}

	createdDir, err := ensureDir(filepath.Join(vaultDir, "tasks"))
	if err != nil {
		return res, err
	}
	note(createdDir, "tasks/")

	templateContent, err := scaffoldTemplateFS.ReadFile("scaffold_template.md")
	if err != nil {
		return res, err
	}
	createdFile, err := ensureFile(filepath.Join(vaultDir, "templates", "task-template.md"), templateContent)
	if err != nil {
		return res, err
	}
	note(createdFile, "templates/task-template.md")

	schemaJSON, err := model.DefaultSchemaJSON()
	if err != nil {
		return res, err
	}
	createdFile, err = ensureFile(filepath.Join(vaultDir, ".task-tracker", "schema.json"), schemaJSON)
	if err != nil {
		return res, err
	}
	note(createdFile, ".task-tracker/schema.json")

	project := opts.Project
	if project == "" {
		project = defaultProjectName(vaultDir)
	}
	prefix := opts.IDPrefix
	if prefix == "" {
		prefix = defaultIDPrefix(project)
	}
	createdFile, err = ensureFile(
		filepath.Join(vaultDir, "projects", project+".md"),
		[]byte(projectFileContent(project, prefix)),
	)
	if err != nil {
		return res, err
	}
	note(createdFile, "projects/"+project+".md")

	return res, nil
}

// ensureDir создаёт каталог, если его ещё нет. Сообщает, был ли он создан
// именно этим вызовом.
func ensureDir(path string) (bool, error) {
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("%s уже существует и не является каталогом", path)
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false, err
	}
	return true, nil
}

// ensureFile пишет файл, только если его ещё нет — существующий файл не
// перезаписывается ни байтом.
func ensureFile(path string, content []byte) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// defaultProjectName — имя первого проекта, если его не назвали явно: имя
// каталога vault, приведённое к тому же kebab-case, что и слаги тасок.
func defaultProjectName(vaultDir string) string {
	base := strings.ToLower(filepath.Base(filepath.Clean(vaultDir)))
	slug := strings.Trim(nonSlugRe.ReplaceAllString(base, "-"), "-")
	if slug == "" {
		slug = "vault"
	}
	return slug
}

// defaultIDPrefix выводит префикс ID из имени проекта: инициалы слов через
// дефис (как OVH у "webapp", TT у "task-tracker" в живом vault), а для
// однословного имени — первые буквы. Это только стартовая догадка — тот, кому
// не подходит, передаёт свой через --id-prefix.
func defaultIDPrefix(project string) string {
	var initials strings.Builder
	for _, w := range strings.Split(project, "-") {
		if w != "" {
			initials.WriteString(strings.ToUpper(w[:1]))
		}
	}
	if initials.Len() >= 2 {
		return initials.String()
	}
	up := strings.ToUpper(strings.ReplaceAll(project, "-", ""))
	if len(up) > 3 {
		up = up[:3]
	}
	if up == "" {
		up = "PRJ"
	}
	return up
}

func projectFileContent(project, prefix string) string {
	return fmt.Sprintf("---\nproject: %s\nid_prefix: %s\nnext_id: 1\n---\n\n# %s\n", project, prefix, project)
}
