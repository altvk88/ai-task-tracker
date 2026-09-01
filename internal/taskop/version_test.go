package taskop

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alkulagin-creator/tt/internal/vault"
)

func TestSetIfVersionRequiresBaseVersion(t *testing.T) {
	root := claimFixture(t)
	if _, err := SetIfVersion(root, "ALP-1", "priority", "low", "agent", ""); err == nil {
		t.Fatal("пустой baseVersion обязан отклоняться")
	}
}

func TestSetIfVersionSucceedsOnMatchingVersion(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	v, err := vault.Version(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := SetIfVersion(root, "ALP-1", "priority", "low", "agent", v)
	if err != nil {
		t.Fatalf("SetIfVersion: %v", err)
	}
	if res.Task.Priority != "low" {
		t.Fatalf("Priority = %q", res.Task.Priority)
	}
}

// Ключевой тест задачи: чужая правка между чтением и записью обязана
// отвечать конфликтом, а не тихо перезаписываться.
func TestSetIfVersionConflictsOnStaleVersion(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	staleVersion, err := vault.Version(path)
	if err != nil {
		t.Fatal(err)
	}

	// Кто-то другой (агент, человек в Obsidian) правит файл первым.
	if err := vault.SetField(path, "title", "правка соседа"); err != nil {
		t.Fatal(err)
	}
	before := read(t, path)

	_, err = SetIfVersion(root, "ALP-1", "priority", "low", "agent", staleVersion)
	if err == nil {
		t.Fatal("устаревший baseVersion обязан давать конфликт")
	}
	kind, ok := KindOf(err)
	if !ok || kind != KindRejected {
		t.Fatalf("вид ошибки = %v (ok=%v), ожидался KindRejected", kind, ok)
	}
	if after := read(t, path); after != before {
		t.Fatalf("отклонённая по версии запись изменила файл:\n%s", after)
	}
}

func TestSetBodyRequiresBaseVersion(t *testing.T) {
	root := claimFixture(t)
	if _, err := SetBody(root, "ALP-1", "новое тело", ""); err == nil {
		t.Fatal("пустой baseVersion обязан отклоняться")
	}
}

func TestSetBodySucceedsOnMatchingVersion(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	v, err := vault.Version(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SetBody(root, "ALP-1", "## Новое тело\n\nПравка панели.\n", v); err != nil {
		t.Fatalf("SetBody: %v", err)
	}
	body, err := vault.Body([]byte(read(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Правка панели") {
		t.Fatalf("тело не записалось: %q", body)
	}
	if !strings.Contains(read(t, path), "id: ALP-1") {
		t.Fatal("фронтматтер потерян")
	}
}

func TestSetBodyConflictsOnStaleVersion(t *testing.T) {
	root := claimFixture(t)
	path := filepath.Join(root, "tasks", "alpha", "one.md")
	staleVersion, err := vault.Version(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.SetField(path, "title", "правка соседа"); err != nil {
		t.Fatal(err)
	}
	before := read(t, path)

	_, err = SetBody(root, "ALP-1", "правка панели поверх устаревшего чтения", staleVersion)
	if err == nil {
		t.Fatal("устаревший baseVersion обязан давать конфликт")
	}
	kind, ok := KindOf(err)
	if !ok || kind != KindRejected {
		t.Fatalf("вид ошибки = %v (ok=%v), ожидался KindRejected", kind, ok)
	}
	if after := read(t, path); after != before {
		t.Fatalf("отклонённая по версии запись тела изменила файл:\n%s", after)
	}
}

func TestSetBodyNotFound(t *testing.T) {
	root := claimFixture(t)
	if _, err := SetBody(root, "ALP-404", "тело", "любая-версия"); err == nil {
		t.Fatal("несуществующая таска обязана давать ошибку")
	} else if kind, ok := KindOf(err); !ok || kind != KindNotFound {
		t.Fatalf("вид ошибки = %v (ok=%v), ожидался KindNotFound", kind, ok)
	}
}
