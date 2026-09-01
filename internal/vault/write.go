package vault

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Сколько раз и с какой паузой повторять rename при ошибке доступа. На NTFS
// сторонний процесс (индексатор Windows, антивирус, открытый в Obsidian файл)
// может коротко держать целевой файл, и rename поверх него возвращает
// ACCESS_DENIED, а не «файл занят» — хотя занятость временная и проходит сама
// за миллисекунды. Суммарная пауза та же, что и у замка в lock.go: с запасом
// хватает пережить чужую блокировку, и мало, чтобы заметно задержать команду
// при настоящей проблеме с правами.
const (
	renameAttempts   = 10
	renameRetryPause = 5 * time.Millisecond
)

// SetField переписывает ровно один ключ фронтматтера, не трогая остальные байты
// файла. Значение подаётся логическим (без кавычек) — квотирование делает писатель.
// Пустое value пишется как "key:" и снимает вложенный блок этого ключа, если он был.
//
// Никакого round-trip через YAML-библиотеку: она переставила бы ключи и потеряла
// пустые значения, превратив git-diff в мусор.
func SetField(path, key, value string) error {
	newLine := key + ":"
	if value != "" {
		newLine += " " + quote(value)
	}
	return setLines(path, key, []string{newLine})
}

// SetFieldRaw переписывает ключ буквальным значением без квотирования —
// нужен структурным значениям YAML вроде blocked_by: [TT-001, TT-002],
// которые SetField превратил бы в квотированную строку (needsQuote реагирует
// на ведущий "["). Механика замены строки та же самая, единственный писатель
// в файл; отличается только то, что попадает в новую строку.
func SetFieldRaw(path, key, raw string) error {
	return setLines(path, key, []string{key + ": " + raw})
}

// SetBlock переписывает ключ вложенным блоком: строка "key:" и по строке на
// каждое поле с отступом в два пробела. Значения квотируются так же, как в
// SetField. Поля с пустым значением не пишутся вовсе: строка "  branch:" в
// блоке читается как null и только зашумляет файл.
//
// Нужен claim'у — единственному ключу фронтматтера с вложенной структурой.
// SetField писать его не умеет: он вложенный блок снимает, а не создаёт.
// Гарантия та же самая — трогаются только строки этого ключа.
func SetBlock(path, key string, fields [][2]string) error {
	lines := []string{key + ":"}
	for _, f := range fields {
		if f[1] == "" {
			continue
		}
		lines = append(lines, "  "+f[0]+": "+quote(f[1]))
	}
	return setLines(path, key, lines)
}

// setLines ищет ключ верхнего уровня во фронтматтере и заменяет его строку
// (вместе со вложенным блоком, если он был) на newLines.
func setLines(path, key string, newLines []string) error {
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

	out := make([]string, 0, len(lines)+1)
	replaced := false
	for i := 0; i < len(lines); i++ {
		if replaced || i == 0 || i >= end || !isTopLevelKey(lines[i], key) {
			out = append(out, lines[i])
			continue
		}
		out = append(out, newLines...)
		replaced = true
		// Пропускаем вложенный блок, принадлежавший этому ключу.
		for i+1 < end && isIndented(lines[i+1]) {
			i++
		}
	}
	if !replaced {
		// Ключа не было — вставляем последней строкой блока, перед закрывающим фенсом.
		out = slices.Insert(out, end, newLines...)
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
	if err := renameAtomic(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// renameAtomic переносит tmp поверх path с повтором на ошибку доступа —
// см. константы renameAttempts/renameRetryPause. Ошибки, не связанные с
// занятостью файла (например, путь не существует), возвращаются сразу же,
// без ожидания.
func renameAtomic(tmp, path string) error {
	var lastErr error
	for attempt := 0; attempt < renameAttempts; attempt++ {
		err := os.Rename(tmp, path)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrPermission) {
			return err
		}
		lastErr = err
		time.Sleep(renameRetryPause)
	}
	return fmt.Errorf("%w (похоже, файл занят другим процессом — Obsidian, антивирус или индексатор Windows)", lastErr)
}
