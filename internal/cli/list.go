package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/alkulagin-creator/tt/internal/model"
	"github.com/alkulagin-creator/tt/internal/vault"
)

// ListOptions — фильтры вывода.
type ListOptions struct {
	Project string
	Status  string
	JSON    bool
}

type listRow struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Project  string `json:"project"`
	Priority string `json:"priority"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	ParseErr string `json:"parseError,omitempty"`
}

// List печатает таски vault с учётом фильтров. Статус в фильтре и статус в
// файле сравниваются в каноническом виде, поэтому --status in-progress находит
// и таски с историческим in_progress. Битая таска (ParseErr не пуст) не имеет
// каноничного статуса и проекта, поэтому при активном фильтре --status или
// --project она отсеивается вместе с несовпавшими; без фильтров видна всегда,
// с пометкой BROKEN вместо заголовка.
func List(w io.Writer, vaultDir string, opt ListOptions) error {
	schema, err := model.LoadSchema(SchemaPath(vaultDir))
	if err != nil {
		return err
	}
	tasks, err := vault.Scan(vaultDir)
	if err != nil {
		return err
	}

	wantStatus := ""
	if opt.Status != "" {
		canon, known := schema.Normalize(opt.Status)
		if !known {
			return fmt.Errorf("неизвестный статус %q", opt.Status)
		}
		wantStatus = canon
	}

	rows := make([]listRow, 0, len(tasks))
	for _, t := range tasks {
		status, _ := schema.Normalize(t.Status)
		if wantStatus != "" && status != wantStatus {
			continue
		}
		if opt.Project != "" && t.Project != opt.Project {
			continue
		}
		rows = append(rows, listRow{
			ID: t.ID, Status: status, Project: t.Project,
			Priority: t.Priority, Title: t.Title, Path: t.Path, ParseErr: t.ParseErr,
		})
	}

	if opt.JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		title := r.Title
		if r.ParseErr != "" {
			title = "BROKEN: " + r.ParseErr
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Status, r.Project, r.Priority, title)
	}
	return tw.Flush()
}
