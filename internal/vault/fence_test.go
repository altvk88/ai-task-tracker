package vault

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// readTemp читает файл целиком, байт-в-байт.
func readTemp(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение %s: %v", path, err)
	}
	return string(raw)
}

const brokenLF = `---
id: SHP-001
title: Тестовая таска
status: ready
claim:
  agent: tt
  host: pc
## Description

Тело таски.

## Log

- 2026-08-31: запись
`

func TestRestoreFenceВставляетФенс(t *testing.T) {
	path := writeTemp(t, brokenLF)

	if err := RestoreFence(path); err != nil {
		t.Fatalf("RestoreFence: %v", err)
	}

	got := readTemp(t, path)
	if !strings.Contains(got, "  host: pc\n---\n\n## Description") {
		t.Errorf("фенс вставлен не перед заголовком:\n%s", got)
	}
	if !strings.Contains(got, "Тело таски.") || !strings.Contains(got, "- 2026-08-31: запись") {
		t.Errorf("тело таски потеряно:\n%s", got)
	}

	task, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("после починки файл не разбирается: %v", err)
	}
	if task.ID != "SHP-001" || task.Title != "Тестовая таска" {
		t.Errorf("поля поплыли: id=%q title=%q", task.ID, task.Title)
	}
}

func TestRestoreFenceСохраняетCRLF(t *testing.T) {
	path := writeTemp(t, strings.ReplaceAll(brokenLF, "\n", "\r\n"))

	if err := RestoreFence(path); err != nil {
		t.Fatalf("RestoreFence: %v", err)
	}

	got := readTemp(t, path)
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("не все переводы строк остались CRLF:\n%q", got)
	}
	if !strings.Contains(got, "  host: pc\r\n---\r\n\r\n## Description") {
		t.Errorf("фенс вставлен не перед заголовком:\n%q", got)
	}
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("после починки файл не разбирается: %v", err)
	}
}

func TestRestoreFenceСохраняетBOM(t *testing.T) {
	path := writeTemp(t, string(bom)+brokenLF)

	if err := RestoreFence(path); err != nil {
		t.Fatalf("RestoreFence: %v", err)
	}

	got := readTemp(t, path)
	if !strings.HasPrefix(got, string(bom)+"---\n") {
		t.Errorf("BOM потерян:\n%q", got[:20])
	}
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("после починки файл не разбирается: %v", err)
	}
}

func TestRestoreFenceОтказПриСтрокеЛога(t *testing.T) {
	const src = `---
id: CRM-001
status: done
claim:
- 2026-07-28: запись лога

## Description

Тело.
`
	path := writeTemp(t, src)

	err := RestoreFence(path)
	if err == nil {
		t.Fatal("ожидался отказ на строке лога перед заголовком")
	}
	if !strings.Contains(err.Error(), "не похожа на фронтматтер") {
		t.Errorf("невнятная ошибка: %v", err)
	}
	if got := readTemp(t, path); got != src {
		t.Errorf("файл изменён при отказе:\n%q", got)
	}
}

func TestRestoreFenceОтказБезЗаголовка(t *testing.T) {
	const src = `---
id: CRM-002
status: done
priority: low
`
	path := writeTemp(t, src)

	if err := RestoreFence(path); !errors.Is(err, ErrNoHeading) {
		t.Errorf("ожидался ErrNoHeading, получено: %v", err)
	}
	if got := readTemp(t, path); got != src {
		t.Errorf("файл изменён при отказе:\n%q", got)
	}
}

func TestRestoreFenceОтказПриЦеломФенсе(t *testing.T) {
	const src = `---
id: SHP-002
status: ready
---

## Description

Тело.
`
	path := writeTemp(t, src)

	if err := RestoreFence(path); !errors.Is(err, ErrFenceIntact) {
		t.Errorf("ожидался ErrFenceIntact, получено: %v", err)
	}
	if got := readTemp(t, path); got != src {
		t.Errorf("файл изменён при отказе:\n%q", got)
	}
}

func TestRestoreFenceОтказБезОткрывающегоФенса(t *testing.T) {
	const src = `# Заметка

## Description

Тело.
`
	path := writeTemp(t, src)

	if err := RestoreFence(path); !errors.Is(err, ErrNoFrontmatter) {
		t.Errorf("ожидался ErrNoFrontmatter, получено: %v", err)
	}
	if got := readTemp(t, path); got != src {
		t.Errorf("файл изменён при отказе:\n%q", got)
	}
}
