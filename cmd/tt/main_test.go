package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestServeGracefulShutdown проверяет, что отмена ctx (её шлёт Ctrl+C через
// signal.NotifyContext в main) останавливает serve без зависания: и HTTP,
// и index.Watch должны развернуться за разумное время. Реальный Ctrl+C из
// автотеста не послать, поэтому проверяется тот же путь кода — отмена ctx —
// напрямую.
func TestServeGracefulShutdown(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, vaultDir, "127.0.0.1:0", "web") }()

	// Дать serve время подняться, прежде чем просить его остановиться.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve вернул ошибку после отмены ctx: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve не завершился за 5с после отмены ctx — похоже на зависание")
	}
}
