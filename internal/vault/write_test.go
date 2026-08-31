package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diffLines считает, сколько строк различается у двух версий файла.
func diffLines(t *testing.T, before, after string) int {
	t.Helper()
	b := strings.Split(before, "\n")
	a := strings.Split(after, "\n")
	if len(b) != len(a) {
		t.Fatalf("изменилось число строк: было %d, стало %d", len(b), len(a))
	}
	n := 0
	for i := range b {
		if b[i] != a[i] {
			n++
		}
	}
	return n
}

const sampleTask = `---
id: WEB-150
title: "Заголовок таски"
status: backlog
project: webapp
priority: medium
due:
created: 2026-08-31
completed:
blocked_by: [WEB-148]
effort: M
attempts: 0
claim:
---

## Description

Тело таски. Строка со словом status: тут не должна пострадать.

## Log

- 2026-08-31: Created
`

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSetFieldChangesExactlyOneLine(t *testing.T) {
	p := writeTemp(t, sampleTask)
	if err := SetField(p, "status", "ready"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := diffLines(t, sampleTask, string(after)); n != 1 {
		t.Fatalf("изменилось строк: %d, обязана меняться ровно одна\n---\n%s", n, after)
	}
	if !strings.Contains(string(after), "\nstatus: ready\n") {
		t.Error("статус не записан")
	}
	if !strings.Contains(string(after), "Строка со словом status: тут не должна пострадать") {
		t.Error("тронуто тело таски")
	}
}

func TestSetFieldEmptyValue(t *testing.T) {
	p := writeTemp(t, sampleTask)
	if err := SetField(p, "claim", ""); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, _ := os.ReadFile(p)
	if !strings.Contains(string(after), "\nclaim:\n") {
		t.Errorf("пустое значение обязано писаться как \"claim:\" без пробела, получено:\n%s", after)
	}
}

func TestSetFieldQuotesDangerousValue(t *testing.T) {
	p := writeTemp(t, sampleTask)
	if err := SetField(p, "title", "Баг (прод): всё сломалось"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, _ := os.ReadFile(p)
	if !strings.Contains(string(after), `title: "Баг (прод): всё сломалось"`) {
		t.Errorf("значение с \": \" обязано быть закавычено, получено:\n%s", after)
	}
	got, err := Parse(after)
	if err != nil {
		t.Fatalf("после записи файл перестал парситься: %v", err)
	}
	if got.Title != "Баг (прод): всё сломалось" {
		t.Errorf("Title = %q", got.Title)
	}
}

func TestSetFieldPreservesCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(sampleTask, "\n", "\r\n")
	p := writeTemp(t, crlf)
	if err := SetField(p, "status", "ready"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, _ := os.ReadFile(p)
	if strings.Contains(strings.ReplaceAll(string(after), "\r\n", ""), "\n") {
		t.Error("часть переводов строк осталась без \\r — файл стал смешанным")
	}
	if n := diffLines(t, crlf, string(after)); n != 1 {
		t.Fatalf("на CRLF изменилось строк: %d", n)
	}
}

func TestSetFieldPreservesBOM(t *testing.T) {
	p := writeTemp(t, "\ufeff"+sampleTask)
	if err := SetField(p, "status", "ready"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(after), "\ufeff") {
		t.Error("BOM потерян")
	}
}

func TestSetFieldReplacesClaimBlock(t *testing.T) {
	withClaim := strings.Replace(sampleTask, "claim:\n", "claim:\n  agent: claude\n  host: DESKTOP\n  branch: avk\n  started: 2026-08-30\n", 1)
	p := writeTemp(t, withClaim)
	if err := SetField(p, "claim", ""); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, _ := os.ReadFile(p)
	if strings.Contains(string(after), "agent: claude") {
		t.Errorf("вложенный блок claim не снят:\n%s", after)
	}
	got, err := Parse(after)
	if err != nil {
		t.Fatalf("файл перестал парситься: %v", err)
	}
	if got.Claimed() {
		t.Error("таска всё ещё считается занятой")
	}
	if got.Status != "backlog" {
		t.Errorf("Status = %q, соседние ключи не должны пострадать", got.Status)
	}
}

func TestSetFieldMissingKeyIsInserted(t *testing.T) {
	p := writeTemp(t, sampleTask)
	if err := SetField(p, "ready_at", "2026-09-01"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	after, _ := os.ReadFile(p)
	got, err := Parse(after)
	if err != nil {
		t.Fatalf("файл перестал парситься: %v", err)
	}
	if got.ReadyAt != "2026-09-01" {
		t.Errorf("ReadyAt = %q", got.ReadyAt)
	}
	if !strings.Contains(string(after), "## Description") {
		t.Error("тело таски потеряно")
	}
}

func TestSetFieldRejectsFileWithoutFrontmatter(t *testing.T) {
	p := writeTemp(t, "# Просто заметка\n")
	if err := SetField(p, "status", "ready"); err == nil {
		t.Fatal("файл без фронтматтера обязан давать ошибку, а не получать новый блок")
	}
}
