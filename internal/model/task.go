// Package model — правила флоу и данные. Никакого ввода-вывода: этот пакет
// компилируется и тестируется без файловой системы и без HTTP.
package model

// Claim — блок claim во фронтматтере, присутствует только у in-progress тасок.
type Claim struct {
	Agent   string `yaml:"agent"`
	Host    string `yaml:"host"`
	Branch  string `yaml:"branch"`
	Started string `yaml:"started"`
}

// Task — таска как она лежит во фронтматтере. Даты остаются строками:
// на этом этапе с ними ничего не вычисляется, парсинг появится там, где
// действительно понадобится сравнение (скрытие старых done на веб-доске).
type Task struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Status    string   `yaml:"status"`
	Project   string   `yaml:"project"`
	Priority  string   `yaml:"priority"`
	Due       string   `yaml:"due"`
	Created   string   `yaml:"created"`
	Completed string   `yaml:"completed"`
	ReadyAt   string   `yaml:"ready_at"`
	Effort    string   `yaml:"effort"`
	Attempts  int      `yaml:"attempts"`
	BlockedBy []string `yaml:"blocked_by"`
	Verify    []string `yaml:"verify"`
	Spec      string   `yaml:"spec"`
	Result    string   `yaml:"result"`
	Claim     *Claim   `yaml:"claim"`

	// Служебные поля, во фронтматтер не пишутся.
	Path     string `yaml:"-"`
	ParseErr string `yaml:"-"`
}

// Claimed сообщает, занята ли таска: пустой ключ claim: даёт nil.
func (t Task) Claimed() bool { return t.Claim != nil && t.Claim.Agent != "" }
