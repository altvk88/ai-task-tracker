package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alkulagin-creator/tt/internal/model"
)

// Scan обходит <vault>/tasks/*/*.md и возвращает все таски, включая битые:
// у них заполнен ParseErr и Path, чтобы их можно было увидеть и починить.
// Каталоги, начинающиеся с _, считаются образцами и пропускаются.
func Scan(vaultDir string) ([]model.Task, error) {
	tasksDir := filepath.Join(vaultDir, "tasks")
	info, err := os.Stat(tasksDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("не найден каталог тасок %s", tasksDir)
	}

	var out []model.Task
	err = filepath.WalkDir(tasksDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != tasksDir && strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		task, parseErr := Parse(raw)
		task.Path = path
		if parseErr != nil {
			task.ParseErr = parseErr.Error()
		}
		out = append(out, task)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ByID индексирует таски по ID; битые без ID в индекс не попадают.
func ByID(tasks []model.Task) map[string]model.Task {
	m := make(map[string]model.Task, len(tasks))
	for _, t := range tasks {
		if t.ID != "" {
			m[t.ID] = t
		}
	}
	return m
}
