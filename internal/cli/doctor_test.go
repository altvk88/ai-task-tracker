package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nofenceTask — таска с потерянным закрывающим фенсом: до заголовка только
// фронтматтер, RestoreFence такую чинит.
const nofenceTask = `---
id: ALP-14
title: без фенса
status: ready
project: alpha
priority: low
created: 2026-08-01

## Description

Тело.
`

// nofenceLogTask — тот же дефект, но перед заголовком строка лога: фенс встал
// бы не туда, поэтому починка обязана отказаться.
const nofenceLogTask = `---
id: ALP-15
title: без фенса, со строкой лога
status: ready
project: alpha
priority: low
created: 2026-08-01
claim:
- 2026-08-01: запись

## Description

Тело.
`

func brokenVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		// незакавыченный заголовок с ": " — YAML не парсится
		"tasks/alpha/broken.md": "---\nid: ALP-9\ntitle: Баг (прод): всё сломалось\nstatus: ready\nproject: alpha\npriority: high\ncreated: 2026-08-01\n---\n",
		// историческое написание статуса
		"tasks/alpha/legacy.md": "---\nid: ALP-10\ntitle: старая\nstatus: in_progress\nproject: alpha\npriority: low\ncreated: 2026-08-01\n---\n",
		// claim у таски вне in-progress
		"tasks/alpha/stale.md": "---\nid: ALP-11\ntitle: залипшая\nstatus: ready\nproject: alpha\npriority: low\ncreated: 2026-08-01\nclaim:\n  agent: claude\n  host: DESK\n  branch: avk\n  started: 2026-08-01\n---\n",
		// ссылка на несуществующий блокер
		"tasks/alpha/dangling.md": "---\nid: ALP-12\ntitle: висячая\nstatus: backlog\nproject: alpha\npriority: low\ncreated: 2026-08-01\nblocked_by: [ALP-999]\n---\n",
		// нет обязательного поля priority
		"tasks/alpha/nofield.md": "---\nid: ALP-13\ntitle: без приоритета\nstatus: ready\nproject: alpha\ncreated: 2026-08-01\n---\n",
		// дубль ID
		"tasks/beta/dup.md": "---\nid: ALP-10\ntitle: дубль\nstatus: ready\nproject: beta\npriority: low\ncreated: 2026-08-01\n---\n",
		// потерян закрывающий фенс, до заголовка только фронтматтер — чинится
		"tasks/alpha/nofence.md": nofenceTask,
		// потерян закрывающий фенс, но перед заголовком строка лога — не чинится
		"tasks/alpha/nofencelog.md": nofenceLogTask,
		// историческая форма verify — НЕ проблема, ругаться нельзя
		"tasks/beta/histverify.md": "---\nid: BET-7\ntitle: со скалярным verify\nstatus: ready\nproject: beta\npriority: low\ncreated: 2026-08-01\nverify: \"pnpm -r typecheck 6/6\"\n---\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// осиротевший замок: таски ALP-777 не существует
	if err := os.MkdirAll(filepath.Join(root, ".locks", "ALP-777.lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDoctorReportsEverything(t *testing.T) {
	root := brokenVault(t)
	var buf bytes.Buffer

	n, err := Doctor(&buf, root, false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if n == 0 {
		t.Fatal("Doctor не нашёл ни одной проблемы в заведомо битом vault")
	}

	out := buf.String()
	for _, want := range []string{"ALP-9", "ALP-10", "ALP-11", "ALP-12", "ALP-13", "ALP-777"} {
		if !strings.Contains(out, want) {
			t.Errorf("в отчёте нет %s:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "дубл") && !strings.Contains(out, "Дубл") {
		t.Errorf("дубль ID не отмечен:\n%s", out)
	}
}

func TestDoctorIgnoresHistoricalForms(t *testing.T) {
	root := brokenVault(t)
	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "BET-7") {
		t.Errorf("скалярный verify — историческая форма, а не проблема; ругаться нельзя:\n%s", buf.String())
	}
}

func TestDoctorWithoutFixChangesNothing(t *testing.T) {
	root := brokenVault(t)
	before := readTask(t, root, "tasks/alpha/legacy.md")

	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, false); err != nil {
		t.Fatal(err)
	}
	if after := readTask(t, root, "tasks/alpha/legacy.md"); after != before {
		t.Error("Doctor без --fix не имеет права менять файлы")
	}
	if _, err := os.Stat(filepath.Join(root, ".locks", "ALP-777.lock")); err != nil {
		t.Error("Doctor без --fix не имеет права снимать замки")
	}
}

func TestDoctorMarksFixable(t *testing.T) {
	root := brokenVault(t)
	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "*") {
		t.Errorf("починяемые проблемы не помечены звёздочкой:\n%s", out)
	}
	// Дубль ID и висячий blocked_by чинить нельзя — они не должны быть помечены.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "ALP-999") && strings.HasPrefix(strings.TrimSpace(line), "*") {
			t.Errorf("висячий blocked_by помечен как починяемый, а он не чинится: %q", line)
		}
	}
}

func TestDoctorCleanVault(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "tasks", "alpha", "ok.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: ALP-1\ntitle: нормальная\nstatus: ready\nproject: alpha\npriority: high\ncreated: 2026-08-01\n---\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	n, err := Doctor(&buf, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("на чистом vault найдено %d проблем:\n%s", n, buf.String())
	}
}

func TestDoctorОтдельнаяКатегорияНезакрытогоФенса(t *testing.T) {
	root := brokenVault(t)
	var buf bytes.Buffer
	if _, err := Doctor(&buf, root, false); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		switch {
		case strings.Contains(line, "ALP-14"):
			if !strings.HasPrefix(line, "* ") || !strings.Contains(line, "фронтматтер не закрыт") {
				t.Errorf("чинимый незакрытый фенс должен быть отдельной категорией со звёздочкой: %q", line)
			}
		case strings.Contains(line, "ALP-15"):
			if strings.HasPrefix(line, "* ") {
				t.Errorf("нечинимый файл помечен звёздочкой: %q", line)
			}
			if !strings.Contains(line, "не похожа на фронтматтер") {
				t.Errorf("в отчёте нет причины, по которой файл не чинится: %q", line)
			}
		}
	}
}
