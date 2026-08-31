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

// issue — одна найденная проблема. Fix != nil означает, что --fix её починит.
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

// Doctor обходит vault и печатает отчёт о найденных проблемах. Без fix не
// пишет ни одного файла и не снимает ни одного замка; с fix применяет только
// механические починки (см. issue.Fix), остальное оставляет человеку.
// Возвращает число найденных проблем.
func Doctor(w io.Writer, vaultDir string, fix bool) (int, error) {
	issues, err := collectIssues(vaultDir)
	if err != nil {
		return 0, err
	}

	if !fix {
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

	fixed := 0
	for _, is := range issues {
		if is.Fix == nil {
			fmt.Fprintf(w, "  %s: %s\n", is.Subject, is.Text)
			continue
		}
		// Неудача одной починки не должна ронять остальные: каждая работает
		// со своим файлом и от прочих не зависит.
		if err := is.Fix(); err != nil {
			fmt.Fprintf(w, "! %s: не удалось починить (%s): %s\n", is.Subject, is.Text, oneLine(err.Error()))
			continue
		}
		fixed++
		fmt.Fprintf(w, "+ %s: %s — починено\n", is.Subject, is.Text)
	}
	fmt.Fprintf(w, "итого: %d проблем, починено %d\n", len(issues), fixed)
	return len(issues), nil
}

// collectIssues собирает все проблемы vault в порядке, воспроизводимом между
// запусками: сортировка по Subject.
func collectIssues(vaultDir string) ([]issue, error) {
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return nil, err
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	issues = append(issues, lockIssues...)

	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Subject < issues[j].Subject })
	return issues, nil
}

// taskIssues — все проблемы одной таски: битый фронтматтер отсекает
// остальные проверки, потому что без разобранных полей их нельзя выполнить.
func taskIssues(schema *model.Schema, t model.Task, byID map[string]model.Task) []issue {
	if t.ParseErr != "" {
		subject := recoverID(t.Path)
		if subject == "" {
			subject = filepath.Base(t.Path)
		}
		return []issue{{
			Subject: subject,
			Text:    "фронтматтер не разбирается: " + oneLine(t.ParseErr),
			Fix:     quoteFix(t.Path),
		}}
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

// field — пара ключ/значение строки фронтматтера, которую надо перезаписать.
type field struct{ key, value string }

// quoteFix возвращает починку незакавыченного значения с ": " внутри — самой
// частой причины неразбираемого фронтматтера. Если во фронтматтере нет такой
// строки (или он вовсе не закрыт), возвращает nil: остальные виды порчи
// требуют человека, а не автозамены.
func quoteFix(path string) func() error {
	fields := unquotedFields(path)
	if len(fields) == 0 || !quotingHelps(path, fields) {
		return nil
	}
	return func() error { return applyQuoting(path, fields) }
}

// quotingHelps примеряет починку на копии в системном temp. Проверка нужна уже
// на этапе отчёта: звёздочка обещает, что --fix справится, а закавычивание
// спасает не всякий битый фронтматтер (в живом vault 8 таск сломаны иначе —
// result записан мапой). Сам vault при этом не трогается.
func quotingHelps(path string, fields []field) bool {
	probe, err := os.CreateTemp("", "tt-doctor-probe-*.md")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	defer os.Remove(name)
	return quoteInto(name, path, fields) == nil
}

// unquotedFields ищет во фронтматтере строки верхнего уровня "ключ: значение",
// где значение само содержит ": " и не закавычено. Строки с вложенным блоком
// пропускаются: SetField снёс бы этот блок вместе со значением.
func unquotedFields(path string) []field {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := strings.TrimPrefix(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !isFenceLine(lines[0]) {
		return nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if isFenceLine(lines[i]) {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}

	var out []field
	for i := 1; i < end; i++ {
		if isIndentedLine(lines[i]) {
			continue
		}
		key, value, ok := strings.Cut(lines[i], ": ")
		if !ok || key == "" || strings.ContainsAny(key, " \t#") {
			continue
		}
		if !strings.Contains(value, ": ") {
			continue
		}
		if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'") {
			continue
		}
		if i+1 < end && isIndentedLine(lines[i+1]) {
			continue
		}
		out = append(out, field{key, strings.TrimSpace(value)})
	}
	return out
}

// applyQuoting переписывает найденные строки на временной копии файла и
// переносит её поверх оригинала, только если после правки файл разбирается.
// Иначе оригинал остаётся байт-в-байт прежним, а doctor печатает неудачу.
func applyQuoting(path string, fields []field) error {
	// Копия лежит рядом с оригиналом, чтобы финальный os.Rename был
	// переносом внутри одного каталога, то есть атомарным.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tt-doctor-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	tmp.Close()

	if err := quoteInto(name, path, fields); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// quoteInto копирует src в dst, закавычивает там перечисленные поля и
// возвращает ошибку, если после этого файл всё ещё не разбирается. Оригинал
// не трогается ни в каком случае.
func quoteInto(dst, src string, fields []field) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return err
	}
	for _, f := range fields {
		if err := vault.SetField(dst, f.key, f.value); err != nil {
			return err
		}
	}
	fixed, err := os.ReadFile(dst)
	if err != nil {
		return err
	}
	if _, err := vault.Parse(fixed); err != nil {
		return fmt.Errorf("после закавычивания файл всё равно не разбирается: %w", err)
	}
	return nil
}

func isFenceLine(line string) bool { return strings.TrimRight(line, " \t\r") == "---" }

func isIndentedLine(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
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
