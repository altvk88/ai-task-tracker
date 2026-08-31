package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestClaimReleaseCycle(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer

	if err := Claim(&buf, root, "ALP-1", "claude"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := readTask(t, root, "tasks/alpha/one.md")
	if !strings.Contains(got, "claim:\n  agent: claude\n") || !strings.Contains(got, "status: in-progress") {
		t.Fatalf("claim не записан:\n%s", got)
	}
	if !strings.Contains(buf.String(), "ready -> in-progress") {
		t.Errorf("вывод claim: %q", buf.String())
	}

	buf.Reset()
	if err := Release(&buf, root, "ALP-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	got = readTask(t, root, "tasks/alpha/one.md")
	if strings.Contains(got, "agent: claude") || !strings.Contains(got, "status: ready") {
		t.Fatalf("release не снял claim:\n%s", got)
	}
}

func TestResetPrintsAttempt(t *testing.T) {
	root := fixtureVault(t)
	var buf bytes.Buffer
	if err := Claim(&buf, root, "ALP-1", "чужой"); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := Reset(&buf, root, "ALP-1"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !strings.Contains(buf.String(), "попытка 1") {
		t.Errorf("вывод reset: %q", buf.String())
	}
}
