// Команда tt — единый инструмент работы с vault таск-трекера.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"

	"github.com/alkulagin-creator/tt/internal/cli"
	"github.com/alkulagin-creator/tt/internal/index"
	"github.com/alkulagin-creator/tt/internal/server"
)

const usage = `tt — таск-трекер над markdown-vault

Использование:
  tt list   [--vault ПУТЬ] [--project ИМЯ] [--status СТАТУС] [--json]
  tt set    [--vault ПУТЬ] [--agent ИМЯ] ID КЛЮЧ [ЗНАЧЕНИЕ]
  tt doctor [--vault ПУТЬ] [--fix]
  tt serve  [--vault ПУТЬ] [--port N] [--listen АДРЕС] [--agent ИМЯ]

Путь к vault берётся из --vault, иначе из переменной TT_VAULT.
`

// defaultServePort — порт tt serve по умолчанию. Не 8080 и не 3000: оба часто
// заняты другими локальными инструментами разработки.
const defaultServePort = 4173

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

	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		fixFlag := fs.Bool("fix", false, "починить найденные проблемы")
		if err := fs.Parse(args); err != nil {
			return err
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		_, err = cli.Doctor(os.Stdout, dir, *fixFlag)
		return err

	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		port := fs.Int("port", defaultServePort, "порт, если не задан --listen")
		listen := fs.String("listen", "", "адрес слушателя (host:port); по умолчанию localhost:--port")
		agent := fs.String("agent", "", "имя писателя для смены статусов из веба")
		if err := fs.Parse(args); err != nil {
			return err
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		addr := *listen
		if addr == "" {
			addr = net.JoinHostPort("localhost", fmt.Sprint(*port))
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return serve(ctx, dir, addr, *agent)

	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil

	default:
		return fmt.Errorf("неизвестная команда %q\n\n%s", cmd, usage)
	}
}

// serve строит индекс, запускает слежение за vault в фоне и поднимает HTTP до
// отмены ctx (Ctrl+C). Живёт в cmd/tt, а не в internal/cli: internal/cli
// импортировал бы internal/server, а тесты internal/server уже импортируют
// internal/cli (для подготовки фикстур через cli.Set) — получился бы цикл.
// Тот же выбор направления зависимостей объяснён в internal/index/index.go.
func serve(ctx context.Context, vaultDir, addr, agent string) error {
	ix, err := index.New(vaultDir)
	if err != nil {
		return err
	}

	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	watchErr := make(chan error, 1)
	go func() { watchErr <- index.Watch(watchCtx, ix, vaultDir) }()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("не удалось занять адрес %s: %w", addr, err)
	}

	srv := server.New(ix, vaultDir, server.Options{Agent: agent})
	httpSrv := &http.Server{Handler: srv.Handler()}

	fmt.Printf("tt serve: http://%s\n", ln.Addr())

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
		// Останавливаем HTTP и ждём завершения вотчера — оба слушают один
		// ctx, поэтому Watch тоже уже разворачивается к возврату.
		_ = httpSrv.Shutdown(context.Background())
		<-watchErr
		return nil
	case err := <-serveErr:
		stopWatch()
		<-watchErr
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}
