package vault

import "strings"

var yamlEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// quote заключает значение в двойные кавычки, если без них YAML прочитает его
// неправильно. Пустое значение не квотируется: ключ пишется как "due:".
func quote(v string) string {
	if !needsQuote(v) {
		return v
	}
	return `"` + yamlEscaper.Replace(v) + `"`
}

func needsQuote(v string) bool {
	if v == "" {
		return false
	}
	if v != strings.TrimSpace(v) {
		return true
	}
	if strings.Contains(v, ": ") || strings.HasSuffix(v, ":") {
		return true
	}
	if strings.ContainsAny(v[:1], "[]{}&*!|>'\"%@`#,?-") {
		return true
	}
	if strings.ContainsAny(v, `"\`) {
		return true
	}
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return true
	}
	return false
}
