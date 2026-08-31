package model

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestVerifyUnmarshal проверяет обе исторические формы поля verify:
// список (актуальная схема) и скаляр (531 таска в живом vault — там
// обычно не команда, а рассказ о результате прогона).
func TestVerifyUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want StringList
	}{
		{
			name: "список из двух элементов",
			yaml: "verify:\n  - go test ./...\n  - go vet ./...\n",
			want: StringList{"go test ./...", "go vet ./..."},
		},
		{
			name: "скаляр",
			yaml: `verify: "-r typecheck 6/6; api 1432/1432"` + "\n",
			want: StringList{"-r typecheck 6/6; api 1432/1432"},
		},
		{
			name: "пустое значение",
			yaml: "verify:\n",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var task Task
			if err := yaml.Unmarshal([]byte(c.yaml), &task); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if len(task.Verify) != len(c.want) {
				t.Fatalf("Verify = %#v, want %#v", task.Verify, c.want)
			}
			for i := range c.want {
				if task.Verify[i] != c.want[i] {
					t.Errorf("Verify[%d] = %q, want %q", i, task.Verify[i], c.want[i])
				}
			}
		})
	}
}

// TestClaimUnmarshal проверяет обе исторические формы claim: блок и скаляр.
func TestClaimUnmarshal(t *testing.T) {
	t.Run("блок", func(t *testing.T) {
		src := "claim:\n  agent: claude\n  host: DESKTOP\n  branch: avk\n  started: 2026-08-30\n"
		var task Task
		if err := yaml.Unmarshal([]byte(src), &task); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !task.Claimed() {
			t.Fatal("Claimed() = false, want true")
		}
		if task.Claim.Agent != "claude" || task.Claim.Host != "DESKTOP" || task.Claim.Branch != "avk" || task.Claim.Started != "2026-08-30" {
			t.Errorf("Claim = %#v", task.Claim)
		}
	})

	t.Run("скаляр", func(t *testing.T) {
		src := `claim: "claude 2026-08-04"` + "\n"
		var task Task
		if err := yaml.Unmarshal([]byte(src), &task); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !task.Claimed() {
			t.Fatal("Claimed() = false, want true")
		}
		if task.Claim.Raw != "claude 2026-08-04" {
			t.Errorf("Claim.Raw = %q", task.Claim.Raw)
		}
	})

	t.Run("пустое значение", func(t *testing.T) {
		src := "claim:\nstatus: backlog\n"
		var task Task
		if err := yaml.Unmarshal([]byte(src), &task); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if task.Claimed() {
			t.Fatal("Claimed() = true, want false")
		}
	})
}

// TestDuplicateKeyIsError — дубли ключей во фронтматтере это порча данных,
// такие файлы обязаны оставаться ошибкой разбора.
func TestDuplicateKeyIsError(t *testing.T) {
	src := "completed: 2026-08-01\ncompleted: 2026-08-02\n"
	var task Task
	err := yaml.Unmarshal([]byte(src), &task)
	if err == nil {
		t.Fatal("дубль ключа обязан быть ошибкой разбора")
	}
	if !strings.Contains(err.Error(), "already defined") {
		t.Errorf("неожиданный текст ошибки: %v", err)
	}
}
