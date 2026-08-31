package model

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

//go:embed schema_default.json
var defaultSchemaFS embed.FS

// Status — один статус и его лейн на доске.
type Status struct {
	ID            string `json:"id"`
	Lane          string `json:"lane"`
	AgentPickable bool   `json:"agentPickable"`
}

// Schema — общий контракт правил флоу. Живёт в vault как
// .task-tracker/schema.json и читается и сервером, и плагином Obsidian,
// поэтому добавленный лейн появляется на обеих досках без правки кода.
type Schema struct {
	Version       int               `json:"version"`
	Statuses      []Status          `json:"statuses"`
	Aliases       map[string]string `json:"aliases"`
	ClearsClaim   []string          `json:"clearsClaim"`
	SetsCompleted []string          `json:"setsCompleted"`
	SetsReadyAt   []string          `json:"setsReadyAt"`
	PromoteFrom   string            `json:"promoteFrom"`
	PromoteTo     string            `json:"promoteTo"`
}

// DefaultSchema возвращает встроенную схему — ей пользуемся, пока в vault нет
// своей, чтобы tt работал на чистой машине без подготовки.
func DefaultSchema() (*Schema, error) {
	raw, err := defaultSchemaFS.ReadFile("schema_default.json")
	if err != nil {
		return nil, err
	}
	return parseSchema(raw)
}

// LoadSchema читает схему из файла; если файла нет, отдаёт встроенную.
func LoadSchema(path string) (*Schema, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultSchema()
	}
	if err != nil {
		return nil, err
	}
	return parseSchema(raw)
}

// DefaultSchemaJSON — байты встроенной схемы, чтобы записать её в vault.
func DefaultSchemaJSON() ([]byte, error) {
	return defaultSchemaFS.ReadFile("schema_default.json")
}

func parseSchema(raw []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("схема не разбирается: %w", err)
	}
	if len(s.Statuses) == 0 {
		return nil, fmt.Errorf("в схеме нет ни одного статуса")
	}
	return &s, nil
}

// Normalize приводит написание статуса к каноническому и сообщает, известен ли он.
// Терпит регистр, пробелы по краям и исторические написания из aliases.
func (s *Schema) Normalize(status string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(status))
	if v == "" {
		return "", false
	}
	if canon, ok := s.Aliases[v]; ok {
		v = canon
	}
	for _, st := range s.Statuses {
		if st.ID == v {
			return v, true
		}
	}
	return strings.TrimSpace(status), false
}

// Lane отдаёт подпись лейна; для неизвестного статуса — сам статус,
// чтобы такая таска была видна на доске, а не потерялась.
func (s *Schema) Lane(id string) string {
	for _, st := range s.Statuses {
		if st.ID == id {
			return st.Lane
		}
	}
	return id
}

// AgentPickable — можно ли брать таску в работу автоматически.
func (s *Schema) AgentPickable(id string) bool {
	for _, st := range s.Statuses {
		if st.ID == id {
			return st.AgentPickable
		}
	}
	return false
}

// ClearsClaimOn сообщает, надо ли снимать claim при переходе в статус.
func (s *Schema) ClearsClaimOn(id string) bool { return slices.Contains(s.ClearsClaim, id) }

// SetsCompletedOn — надо ли проставлять completed.
func (s *Schema) SetsCompletedOn(id string) bool { return slices.Contains(s.SetsCompleted, id) }

// SetsReadyAtOn — надо ли проставлять ready_at.
func (s *Schema) SetsReadyAtOn(id string) bool { return slices.Contains(s.SetsReadyAt, id) }
