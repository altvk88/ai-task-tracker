// Package vault — единственный пакет, который трогает файловую систему vault:
// markdown, фронтматтер, блокировки, атомарная запись.
package vault

import (
	"bytes"
	"errors"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/alkulagin-creator/tt/internal/model"
)

var bom = []byte{0xEF, 0xBB, 0xBF}

// ErrNoFrontmatter — файл не начинается с фенса ---.
var ErrNoFrontmatter = errors.New("нет фронтматтера")

// ErrUnclosedFrontmatter — открывающий фенс есть, закрывающего нет.
var ErrUnclosedFrontmatter = errors.New("фронтматтер не закрыт")

// Split вырезает YAML между открывающим и закрывающим фенсами и нормализует
// переводы строк в \n. Чтение обрывается на закрывающем фенсе, поэтому тело
// таски никогда не парсится — на 1251 файле это ~1-2 МБ вместо десятков.
func Split(src []byte) ([]byte, error) {
	src = bytes.TrimPrefix(src, bom)
	lines := strings.Split(string(src), "\n")
	if len(lines) == 0 || !isFence(lines[0]) {
		return nil, ErrNoFrontmatter
	}
	for i := 1; i < len(lines); i++ {
		if isFence(lines[i]) {
			block := make([]string, 0, i-1)
			for _, l := range lines[1:i] {
				block = append(block, strings.TrimSuffix(l, "\r"))
			}
			return []byte(strings.Join(block, "\n")), nil
		}
	}
	return nil, ErrUnclosedFrontmatter
}

func isFence(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}

// Parse разбирает фронтматтер файла в model.Task. Ошибка парсинга не теряется:
// вызывающий код кладёт её в Task.ParseErr, чтобы таска попала в лоток "Broken",
// а не исчезла из виду (нынешние bash-скрипты такие файлы молча пропускают).
func Parse(src []byte) (model.Task, error) {
	var t model.Task
	block, err := Split(src)
	if err != nil {
		return t, err
	}
	if err := yaml.Unmarshal(block, &t); err != nil {
		return t, err
	}
	return t, nil
}
