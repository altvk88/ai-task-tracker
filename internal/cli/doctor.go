package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// issue — одна найденная проблема. Fix != nil означает, что --fix её починит
// (починка появится в TT-012, здесь только отчёт).
type issue struct {
	Subject string
	Text    string
	Fix     func() error
}

// requiredFields — обязательные поля фронтматтера; проверяются в этом
// порядке, чтобы вывод для одной таски был воспроизводим.
var requiredFields = []struct{ key, label string }{
	{"id", "id"}, {"title", "title"}, {"status", "status"},
	{"project", "project"}, {"priority", "priority"}, {"created", "created"},
}

// Doctor обходит vault и печатает отчёт о найденных проблемах: не пишет ни
// одного файла и не снимает ни одного замка. Возвращает число проблем.
// fix пока не реализован (появится в TT-012) — с ним Doctor честно
// отказывается работать вместо того, чтобы молча ничего не делать.
func Doctor(w io.Writer, vaultDir string, fix bool) (int, error) {
	if fix {
		return 0, fmt.Errorf("починка (--fix) появится в TT-012 — пока doctor умеет только отчёт")
	}

	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return 0, err
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return 0, err
	}

	byID := make(map[string]model.Task, len(tasks))
	dupPaths := make(map[string][]string)
	for _, t := range tasks {
		if t.ID == "" {
			continue
		}
		byID[t.ID] = t
		dupPaths[t.ID] = append(dupPaths[t.ID], t.Path)
	}

	var issues []issue
	for _, t := range tasks {
		issues = append(issues, taskIssues(schema, t, byID)...)
	}
	for id, paths := range dupPaths {
		if len(paths) > 1 {
			issues = append(issues, issue{Subject: id, Text: fmt.Sprintf("дубль ID в файлах: %s", strings.Join(paths, ", "))})
		}
	}
	locksDir := filepath.Join(vaultDir, ".locks")
	lockIssues, err := lockIssues(locksDir, byID, schema)
	if err != nil {
		return 0, err
	}
	issues = append(issues, lockIssues...)

	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Subject < issues[j].Subject })

	fixable := 0
	for _, is := range issues {
		mark := " "
		if is.Fix != nil {
			mark = "*"
			fixable++
		}
		fmt.Fprintf(w, "%s %s: %s\n", mark, is.Subject, is.Text)
	}
	fmt.Fprintf(w, "итого: %d проблем, из них %d чинятся флагом --fix\n", len(issues), fixable)
	return len(issues), nil
}

// taskIssues — все проблемы одной таски: битый фронтматтер отсекает
// остальные проверки, потому что без разобранных полей их нельзя выполнить.
func taskIssues(schema *model.Schema, t model.Task, byID map[string]model.Task) []issue {
	if t.ParseErr != "" {
		subject := recoverID(t.Path)
		if subject == "" {
			subject = filepath.Base(t.Path)
		}
		return []issue{{Subject: subject, Text: "фронтматтер не разбирается: " + oneLine(t.ParseErr)}}
	}

	subject := t.ID
	if subject == "" {
		subject = filepath.Base(t.Path)
	}

	var out []issue
	for _, f := range requiredFields {
		if fieldValue(t, f.key) == "" {
			out = append(out, issue{Subject: subject, Text: fmt.Sprintf("не заполнено обязательное поле %q", f.key)})
		}
	}

	canon, known := schema.Normalize(t.Status)
	switch {
	case !known:
		out = append(out, issue{Subject: subject, Text: fmt.Sprintf("статус %q неизвестен схеме", t.Status)})
	case canon != t.Status:
		path := t.Path
		out = append(out, issue{
			Subject: subject,
			Text:    fmt.Sprintf("статус записан в историческом написании %q (канон %q)", t.Status, canon),
			Fix:     func() error { return vault.SetField(path, "status", canon) },
		})
	}

	if known && canon != "in-progress" && t.Claimed() {
		path := t.Path
		out = append(out, issue{
			Subject: subject,
			Text:    "claim выставлен у таски вне in-progress",
			Fix:     func() error { return vault.SetField(path, "claim", "") },
		})
	}

	for _, blockerID := range t.BlockedBy {
		if blockerID == "" {
			continue
		}
		if _, ok := byID[blockerID]; !ok {
			out = append(out, issue{Subject: subject, Text: fmt.Sprintf("blocked_by ссылается на несуществующую %s", blockerID)})
		}
	}

	return out
}

// fieldValue отдаёт значение обязательного поля по имени, не расширяя model.Task
// геттерами ради одной проверки.
func fieldValue(t model.Task, key string) string {
	switch key {
	case "id":
		return t.ID
	case "title":
		return t.Title
	case "status":
		return t.Status
	case "project":
		return t.Project
	case "priority":
		return t.Priority
	case "created":
		return t.Created
	}
	return ""
}

// lockIssues проверяет каталог .locks: замок должен существовать только пока
// таска в in-progress. Отсутствие .locks — не проблема, а обычное состояние
// свежего vault.
func lockIssues(locksDir string, byID map[string]model.Task, schema *model.Schema) ([]issue, error) {
	entries, err := os.ReadDir(locksDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []issue
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".lock")
		lockPath := filepath.Join(locksDir, e.Name())

		t, ok := byID[id]
		if !ok {
			out = append(out, issue{
				Subject: id,
				Text:    "замок .locks без соответствующей таски",
				Fix:     func() error { return os.Remove(lockPath) },
			})
			continue
		}
		if canon, known := schema.Normalize(t.Status); !known || canon != "in-progress" {
			out = append(out, issue{
				Subject: id,
				Text:    fmt.Sprintf("замок .locks при статусе %q (не in-progress)", t.Status),
				Fix:     func() error { return os.Remove(lockPath) },
			})
		}
	}
	return out, nil
}

// recoverID достаёт id из сырого файла, когда фронтматтер не разобрался
// целиком: без этого у 69 живых битых тасок Subject был бы только именем
// файла, а по нему ID не найти в остальном тексте отчёта.
func recoverID(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, found := strings.CutPrefix(line, "id:"); found {
			return strings.TrimSpace(strings.TrimSuffix(rest, "\r"))
		}
	}
	return ""
}
