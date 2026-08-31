package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextPicksTask(t *testing.T) {
	root := fixtureVault(t) // BET-1 — единственная ready-таска проекта beta
	var buf bytes.Buffer
	if err := Next(&buf, root, "beta", false); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "BET-1") {
		t.Errorf("вывод = %q, ожидался BET-1", got)
	}
}

func TestNextEmptyQueueExitsCleanly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks", "gamma"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Next(&buf, root, "gamma", false); err != nil {
		t.Fatalf("пустая очередь не должна быть ошибкой: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "нечего брать") {
		t.Errorf("вывод = %q, ожидалось внятное сообщение о пустой очереди", got)
	}
}

func TestNextJSONValidOnPick(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer
	if err := Next(&buf, root, "beta", true); err != nil {
		t.Fatal(err)
	}
	var row nextRow
	if err := json.Unmarshal(buf.Bytes(), &row); err != nil {
		t.Fatalf("невалидный JSON: %v\n%s", err, buf.String())
	}
	if row.ID != "BET-1" {
		t.Errorf("ID = %q, ожидался BET-1", row.ID)
	}
}

func TestNextJSONNullOnEmptyQueue(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks", "gamma"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Next(&buf, root, "gamma", true); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "null" {
		t.Errorf("JSON при пустой очереди = %q, ожидался null", got)
	}
}
