package vault

import "testing"

func TestSplit(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
		err  bool
	}{
		{
			name: "простой блок",
			src:  "---\nid: WEB-1\nstatus: ready\n---\n\n## Description\n",
			want: "id: WEB-1\nstatus: ready",
		},
		{
			name: "CRLF",
			src:  "---\r\nid: WEB-1\r\nstatus: ready\r\n---\r\n\r\ntext\r\n",
			want: "id: WEB-1\nstatus: ready",
		},
		{
			name: "BOM перед первым ---",
			src:  "\ufeff---\nid: WEB-1\n---\n",
			want: "id: WEB-1",
		},
		{
			name: "тело содержит --- и не должно попасть в блок",
			src:  "---\nid: WEB-1\n---\n\ntext\n\n---\n\nmore\n",
			want: "id: WEB-1",
		},
		{
			name: "нет фронтматтера",
			src:  "# Просто заметка\n",
			err:  true,
		},
		{
			name: "фенс не закрыт",
			src:  "---\nid: WEB-1\n",
			err:  true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Split([]byte(c.src))
			if c.err {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получен блок %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if string(got) != c.want {
				t.Fatalf("получено %q, ожидалось %q", got, c.want)
			}
		})
	}
}

func TestParseRealTask(t *testing.T) {
	src := "---\n" +
		"id: WEB-150\n" +
		"title: \"`Accept` и `Cancel` в строке, метка `SF sync failed`\"\n" +
		"status: backlog\n" +
		"project: webapp\n" +
		"priority: medium\n" +
		"due:\n" +
		"created: 2026-08-31\n" +
		"blocked_by: [WEB-148]\n" +
		"effort: M\n" +
		"attempts: 0\n" +
		"verify:\n" +
		"  - npm run selftest\n" +
		"claim:\n" +
		"---\n\n## Description\n\nтекст\n"

	got, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got.ID != "WEB-150" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Created != "2026-08-31" {
		t.Errorf("Created = %q, дата обязана остаться строкой", got.Created)
	}
	if len(got.BlockedBy) != 1 || got.BlockedBy[0] != "WEB-148" {
		t.Errorf("BlockedBy = %v", got.BlockedBy)
	}
	if len(got.Verify) != 1 || got.Verify[0] != "npm run selftest" {
		t.Errorf("Verify = %v", got.Verify)
	}
	if got.Claimed() {
		t.Errorf("пустой claim: не должен считаться занятым")
	}
	if got.Due != "" {
		t.Errorf("Due = %q, пустое значение обязано остаться пустым", got.Due)
	}
}
