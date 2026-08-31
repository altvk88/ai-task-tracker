package vault

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ErrFenceIntact — закрывающий фенс на месте, восстанавливать нечего.
var ErrFenceIntact = errors.New("закрывающий фенс на месте")

// ErrNoHeading — в файле нет ни одного заголовка "## ", перед которым можно
// было бы поставить потерянный фенс.
var ErrNoHeading = errors.New("нет заголовка \"## \", перед которым восстановить фенс")

// frontmatterKey — строка вида "ключ:" верхнего уровня.
var frontmatterKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*:`)

// RestoreFence восстанавливает потерянный закрывающий фенс: ставит "---" и
// пустую строку перед первым заголовком "## ".
//
// Чинит только заведомо безопасный случай — когда всё между открывающим фенсом
// и заголовком выглядит как фронтматтер. Если там есть хоть одна посторонняя
// строка (например строка лога "- 2026-07-28: ..."), фенс встал бы не туда и
// часть тела уехала бы во фронтматтер, поэтому файл не трогается вовсе.
func RestoreFence(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	hasBOM := bytes.HasPrefix(raw, bom)
	body := bytes.TrimPrefix(raw, bom)

	eol := "\n"
	if bytes.Contains(body, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")

	if len(lines) == 0 || !isFence(lines[0]) {
		return ErrNoFrontmatter
	}

	heading := -1
	for i := 1; i < len(lines); i++ {
		if isFence(lines[i]) {
			return ErrFenceIntact
		}
		if strings.HasPrefix(lines[i], "## ") {
			heading = i
			break
		}
		if !looksLikeFrontmatter(lines[i]) {
			return fmt.Errorf("строка %d не похожа на фронтматтер: %s", i+1, lines[i])
		}
	}
	if heading < 0 {
		return ErrNoHeading
	}

	out := make([]string, 0, len(lines)+2)
	out = append(out, lines[:heading]...)
	out = append(out, "---", "")
	out = append(out, lines[heading:]...)

	var data bytes.Buffer
	if hasBOM {
		data.Write(bom)
	}
	data.WriteString(strings.Join(out, eol))

	if _, err := Parse(data.Bytes()); err != nil {
		return fmt.Errorf("после вставки фенса файл не разбирается: %w", err)
	}
	return writeAtomic(path, data.Bytes())
}

// looksLikeFrontmatter — строка допустима внутри блока фронтматтера: ключ
// верхнего уровня, продолжение вложенного блока или пустая строка.
func looksLikeFrontmatter(line string) bool {
	return line == "" || isIndented(line) || frontmatterKey.MatchString(line)
}
