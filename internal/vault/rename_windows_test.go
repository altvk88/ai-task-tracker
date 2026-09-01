//go:build windows

package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// openWithoutShareDelete открывает файл так, как это делают сторонние
// процессы (индексатор, антивирус, Obsidian) — без FILE_SHARE_DELETE. Именно
// это и превращает os.Rename поверх файла в ACCESS_DENIED на NTFS: сам Go
// при своих открытиях всегда просит FILE_SHARE_DELETE, поэтому обычным
// os.Open эту гонку из теста не воспроизвести — нужен прямой вызов Win32 API.
func openWithoutShareDelete(t *testing.T, path string) windows.Handle {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, // без FILE_SHARE_DELETE
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestRenameAtomicRetriesThroughTransientLock воспроизводит ровно ту гонку из
// таски: целевой файл на короткое время держит сторонний процесс, первая
// попытка rename должна упасть на ACCESS_DENIED, а после освобождения файла —
// пройти без вмешательства вызывающего кода.
func TestRenameAtomicRetriesThroughTransientLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".tt-new.tmp")
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := openWithoutShareDelete(t, path)
	defer windows.CloseHandle(h)

	// Первая попытка обязана упереться в занятость файла.
	if err := os.Rename(tmp, path); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("ожидалась ошибка доступа на занятом файле, получено: %v", err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err) // предыдущий Rename мог частично сработать/откатиться — пересоздаём tmp
	}

	// Отпускаем файл раньше, чем исчерпается бюджет повторов renameAtomic.
	time.AfterFunc(renameRetryPause*2, func() {
		windows.CloseHandle(h)
	})

	if err := renameAtomic(tmp, path); err != nil {
		t.Fatalf("renameAtomic не пережил временную занятость файла: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("файл не переписан: content=%q err=%v", got, err)
	}
}

// TestRenameAtomicGivesUpWithExplainedError проверяет, что при занятости
// файла на весь бюджет повторов renameAtomic не зависает, а возвращает
// ошибку, называющую вероятную причину, а не голое «Access is denied».
func TestRenameAtomicGivesUpWithExplainedError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".tt-new.tmp")
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := openWithoutShareDelete(t, path)
	defer windows.CloseHandle(h)

	err := renameAtomic(tmp, path)
	if err == nil {
		t.Fatal("ожидалась ошибка — файл занят на весь бюджет повторов")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("ошибка должна оборачивать исходную ошибку доступа: %v", err)
	}
	if !strings.Contains(err.Error(), "занят") {
		t.Errorf("ошибка должна называть вероятную причину, получено: %v", err)
	}
}
