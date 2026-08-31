package vault

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SetField переписывает ровно один ключ фронтматтера, не трогая остальные байты
// файла. Значение подаётся логическим (без кавычек) — квотирование делает писатель.
// Пустое value пишется как "key:" и снимает вложенный блок этого ключа, если он был.
//
// Никакого round-trip через YAML-библиотеку: она переставила бы ключи и потеряла
// пустые значения, превратив git-diff в мусор.
func SetField(path, key, value string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	hasBOM := bytes.HasPrefix(raw, bom)
	raw = bytes.TrimPrefix(raw, bom)

	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")

	if len(lines) == 0 || !isFence(lines[0]) {
		return fmt.Errorf("%s: %w", path, ErrNoFrontmatter)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if isFence(lines[i]) {
			end = i
			break
		}
	}
	if end < 0 {
		return fmt.Errorf("%s: %w", path, ErrUnclosedFrontmatter)
	}

	newLine := key + ":"
	if value != "" {
		newLine += " " + quote(value)
	}

	out := make([]string, 0, len(lines)+1)
	replaced := false
	for i := 0; i < len(lines); i++ {
		if replaced || i == 0 || i >= end || !isTopLevelKey(lines[i], key) {
			out = append(out, lines[i])
			continue
		}
		out = append(out, newLine)
		replaced = true
		// Пропускаем вложенный блок, принадлежавший этому ключу.
		for i+1 < end && isIndented(lines[i+1]) {
			i++
		}
	}
	if !replaced {
		// Ключа не было — вставляем последней строкой блока, перед закрывающим фенсом.
		out = slices.Insert(out, end, newLine)
	}

	var data bytes.Buffer
	if hasBOM {
		data.Write(bom)
	}
	data.WriteString(strings.Join(out, eol))
	return writeAtomic(path, data.Bytes())
}

// isTopLevelKey — строка задаёт ключ верхнего уровня с таким именем.
func isTopLevelKey(line, key string) bool {
	if isIndented(line) {
		return false
	}
	return strings.HasPrefix(line, key+":")
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// writeAtomic пишет во временный файл в том же каталоге и переносит его поверх
// оригинала. os.Rename внутри одного каталога NTFS атомарен, поэтому прерванный
// запуск не может оставить недописанный файл таски.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tt-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { f.Close(); os.Remove(tmp) }

	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
