// Команда tt — единый инструмент работы с vault таск-трекера.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/alkulagin-creator/tt/internal/cli"
)

const usage = `tt — таск-трекер над markdown-vault

Использование:
  tt list [--vault ПУТЬ] [--project ИМЯ] [--status СТАТУС] [--json]
  tt set  [--vault ПУТЬ] [--agent ИМЯ] ID КЛЮЧ [ЗНАЧЕНИЕ]

Путь к vault берётся из --vault, иначе из переменной TT_VAULT.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "tt:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	switch cmd {
	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		project := fs.String("project", "", "фильтр по проекту")
		status := fs.String("status", "", "фильтр по статусу")
		asJSON := fs.Bool("json", false, "вывод в JSON")
		if err := fs.Parse(args); err != nil {
			return err
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		return cli.List(os.Stdout, dir, cli.ListOptions{Project: *project, Status: *status, JSON: *asJSON})

	case "set":
		fs := flag.NewFlagSet("set", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		agent := fs.String("agent", "cli", "имя агента для проверки claim")
		if err := fs.Parse(args); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) < 2 {
			return fmt.Errorf("использование: tt set ID КЛЮЧ [ЗНАЧЕНИЕ]")
		}
		value := ""
		if len(rest) > 2 {
			value = rest[2]
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		return cli.Set(os.Stdout, dir, rest[0], rest[1], value, *agent)

	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil

	default:
		return fmt.Errorf("неизвестная команда %q\n\n%s", cmd, usage)
	}
}
