// Package model — правила флоу и данные. Никакого ввода-вывода: этот пакет
// компилируется и тестируется без файловой системы и без HTTP.
package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// StringList — поле, которое исторически писалось и списком, и одной строкой.
// В vault 531 таска держит verify скаляром: там лежит не команда, а рассказ
// о результате прогона. Читаем обе формы, данные не переписываем.
type StringList []string

// UnmarshalYAML принимает список, непустой скаляр (как срез из одного
// элемента) и пустой/null скаляр (как nil).
func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	case yaml.ScalarNode:
		if value.Value == "" {
			*s = nil
			return nil
		}
		*s = StringList{value.Value}
		return nil
	default:
		return fmt.Errorf("неожиданный узел YAML для списка строк: %v", value.Kind)
	}
}

// Claim — блок claim во фронтматтере, присутствует только у in-progress тасок.
// Часть тасок в vault держит claim не блоком, а произвольной строкой
// ("claude 2026-08-04", "avk @ 2026-06-29") — такой текст сохраняется в Raw,
// чтобы не терять информацию о том, что таска занята.
type Claim struct {
	Agent   string `yaml:"agent"`
	Host    string `yaml:"host"`
	Branch  string `yaml:"branch"`
	Started string `yaml:"started"`
	Raw     string `yaml:"-"`
}

// UnmarshalYAML принимает блок с полями agent/host/branch/started и
// непустой скаляр, который целиком сохраняется в Raw.
func (c *Claim) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		type claimAlias Claim
		var a claimAlias
		if err := value.Decode(&a); err != nil {
			return err
		}
		*c = Claim(a)
		return nil
	case yaml.ScalarNode:
		if value.Value == "" {
			return nil
		}
		*c = Claim{Raw: value.Value}
		return nil
	default:
		return fmt.Errorf("неожиданный узел YAML для claim: %v", value.Kind)
	}
}

// Task — таска как она лежит во фронтматтере. Даты остаются строками:
// на этом этапе с ними ничего не вычисляется, парсинг появится там, где
// действительно понадобится сравнение (скрытие старых done на веб-доске).
type Task struct {
	ID        string     `yaml:"id"`
	Title     string     `yaml:"title"`
	Status    string     `yaml:"status"`
	Project   string     `yaml:"project"`
	Priority  string     `yaml:"priority"`
	Due       string     `yaml:"due"`
	Created   string     `yaml:"created"`
	Completed string     `yaml:"completed"`
	ReadyAt   string     `yaml:"ready_at"`
	Effort    string     `yaml:"effort"`
	Attempts  int        `yaml:"attempts"`
	BlockedBy []string   `yaml:"blocked_by"`
	Verify    StringList `yaml:"verify"`
	Spec      string     `yaml:"spec"`
	Result    string     `yaml:"result"`
	Claim     *Claim     `yaml:"claim"`

	// Служебные поля, во фронтматтер не пишутся.
	Path     string `yaml:"-"`
	ParseErr string `yaml:"-"`
}

// Claimed сообщает, занята ли таска: пустой ключ claim: даёт nil.
func (t Task) Claimed() bool {
	return t.Claim != nil && (t.Claim.Agent != "" || t.Claim.Raw != "")
}
