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
	"strings"

	"github.com/alkulagin-creator/tt/internal/cli"
	"github.com/alkulagin-creator/tt/internal/index"
	"github.com/alkulagin-creator/tt/internal/server"
	"github.com/alkulagin-creator/tt/internal/taskop"
)

const usage = `tt — таск-трекер над markdown-vault

Использование:
  tt list   [--vault ПУТЬ] [--project ИМЯ] [--status СТАТУС] [--json]
  tt next   [--vault ПУТЬ] --project ИМЯ [--json]
  tt new    [--vault ПУТЬ] --project ИМЯ --title ТЕКСТ [--priority high|medium|low]
            [--effort 2h] [--depends-on ID,ID] [--spec ПУТЬ]
  tt set    [--vault ПУТЬ] [--agent ИМЯ] ID КЛЮЧ [ЗНАЧЕНИЕ]
  tt claim   [--vault ПУТЬ] [--agent ИМЯ] ID
  tt release [--vault ПУТЬ] ID
  tt reset   [--vault ПУТЬ] ID
  tt done    [--vault ПУТЬ] [--agent ИМЯ] [--result ТЕКСТ] ID
  tt scaffold [--vault ПУТЬ] [--project ИМЯ] [--id-prefix ПРЕФИКС]
  tt doctor [--vault ПУТЬ] [--fix]
  tt serve  [--vault ПУТЬ] [--port N] [--listen АДРЕС] [--agent ИМЯ] [--token ТОКЕН]
  tt config show [--vault ПУТЬ] [--port N]
  tt config set  [--vault ПУТЬ] [--port N]

Путь к vault берётся из --vault, иначе из переменной TT_VAULT, иначе из файла
настроек (см. tt config). Порт tt serve — из --port, иначе из файла настроек,
иначе встроенный дефолт 4173.
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

	case "next":
		fs := flag.NewFlagSet("next", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		project := fs.String("project", "", "проект (обязателен)")
		asJSON := fs.Bool("json", false, "вывод в JSON")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *project == "" {
			return fmt.Errorf("использование: tt next --project ИМЯ [--json]")
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		return cli.Next(os.Stdout, dir, *project, *asJSON)

	case "new":
		fs := flag.NewFlagSet("new", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		project := fs.String("project", "", "проект (обязателен)")
		title := fs.String("title", "", "заголовок таски (обязателен)")
		priority := fs.String("priority", "", "high|medium|low, по умолчанию medium")
		effort := fs.String("effort", "", "оценка трудозатрат, например 2h")
		dependsOn := fs.String("depends-on", "", "ID зависимостей через запятую")
		spec := fs.String("spec", "", "путь к спеке или плану")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *project == "" || *title == "" {
			return fmt.Errorf("использование: tt new --project ИМЯ --title ТЕКСТ [--priority high|medium|low] [--effort 2h] [--depends-on ID,ID] [--spec ПУТЬ]")
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		var deps []string
		if *dependsOn != "" {
			for _, id := range strings.Split(*dependsOn, ",") {
				deps = append(deps, strings.TrimSpace(id))
			}
		}
		return cli.New(os.Stdout, dir, taskop.NewOptions{
			Project:   *project,
			Title:     *title,
			Priority:  *priority,
			Effort:    *effort,
			Spec:      *spec,
			DependsOn: deps,
		})

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

	case "claim", "release", "reset":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		// --agent осмыслен только у claim: release и reset снимают claim, а не
		// пишут его. Регистрировать флаг у них — обещать влияние, которого нет.
		agent := new(string)
		if cmd == "claim" {
			agent = fs.String("agent", "cli", "имя агента, который берёт таску")
		}
		if err := fs.Parse(args); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) != 1 {
			return fmt.Errorf("использование: tt %s ID", cmd)
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		switch cmd {
		case "claim":
			return cli.Claim(os.Stdout, dir, rest[0], *agent)
		case "release":
			return cli.Release(os.Stdout, dir, rest[0])
		default:
			return cli.Reset(os.Stdout, dir, rest[0])
		}

	case "done":
		fs := flag.NewFlagSet("done", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		agent := fs.String("agent", "cli", "имя агента для проверки claim")
		result := fs.String("result", "", "текст поля result")
		if err := fs.Parse(args); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) != 1 {
			return fmt.Errorf("использование: tt done [--result ТЕКСТ] ID")
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		return cli.Done(os.Stdout, dir, rest[0], *result, *agent)

	case "scaffold":
		fs := flag.NewFlagSet("scaffold", flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		project := fs.String("project", "", "имя первого проекта; по умолчанию — имя каталога vault")
		idPrefix := fs.String("id-prefix", "", "префикс ID проекта; по умолчанию выводится из имени проекта")
		if err := fs.Parse(args); err != nil {
			return err
		}
		// Не cli.ResolveVault: тот требует уже существующий каталог tasks,
		// а scaffold как раз создаёт структуру там, где её ещё нет.
		dir, err := cli.ResolveVaultForScaffold(*vaultFlag)
		if err != nil {
			return err
		}
		return cli.Scaffold(os.Stdout, dir, taskop.ScaffoldOptions{Project: *project, IDPrefix: *idPrefix})

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
		token := fs.String("token", "", "токен на запись для запросов не с loopback-адреса; не задан — сгенерируется")
		if err := fs.Parse(args); err != nil {
			return err
		}
		dir, err := cli.ResolveVault(*vaultFlag)
		if err != nil {
			return err
		}
		// resolvedPort — порт с учётом файла настроек. flagSet(...) нужен, потому
		// что у --port уже есть значение по умолчанию: «флаг не передан» и «передан
		// ровно 4173» неразличимы без явной проверки через fs.Visit.
		resolvedPort, _, err := cli.ResolvePort(*port, flagSet(fs, "port"), defaultServePort)
		if err != nil {
			return err
		}
		// --listen принимает и голый адрес ("0.0.0.0"), и полный ("0.0.0.0:4180").
		// Без этого `--listen 0.0.0.0 --port 4179` падал с «missing port in address»:
		// в справке флаги описаны как независимые, значит и сочетаться должны.
		addr := *listen
		switch {
		case addr == "":
			addr = net.JoinHostPort("localhost", fmt.Sprint(resolvedPort))
		case !strings.Contains(addr, ":"):
			addr = net.JoinHostPort(addr, fmt.Sprint(resolvedPort))
		case strings.HasSuffix(addr, ":"):
			addr = net.JoinHostPort(strings.TrimSuffix(addr, ":"), fmt.Sprint(resolvedPort))
		}

		tok := *token
		if tok == "" {
			tok, err = server.GenerateToken()
			if err != nil {
				return fmt.Errorf("не удалось сгенерировать токен: %w", err)
			}
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return serve(ctx, dir, addr, *agent, tok)

	case "config":
		if len(args) == 0 {
			return fmt.Errorf("использование: tt config show|set [--vault ПУТЬ] [--port N]")
		}
		sub, rest := args[0], args[1:]
		fs := flag.NewFlagSet("config "+sub, flag.ExitOnError)
		vaultFlag := fs.String("vault", "", "путь к vault")
		port := fs.Int("port", defaultServePort, "порт tt serve")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		portSet := flagSet(fs, "port")
		switch sub {
		case "show":
			return cli.ConfigShow(os.Stdout, *vaultFlag, *port, portSet)
		case "set":
			return cli.ConfigSet(os.Stdout, *vaultFlag, *vaultFlag != "", *port, portSet)
		default:
			return fmt.Errorf("неизвестная подкоманда %q для tt config: show|set", sub)
		}

	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil

	default:
		return fmt.Errorf("неизвестная команда %q\n\n%s", cmd, usage)
	}
}

// flagSet сообщает, был ли флаг name реально передан в командной строке —
// в отличие от *fs.Int/*fs.String, которые для непереданного флага неотличимы
// от переданного со значением по умолчанию.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// serve строит индекс, запускает слежение за vault в фоне и поднимает HTTP до
// отмены ctx (Ctrl+C). Живёт в cmd/tt, а не в internal/cli: internal/cli
// импортировал бы internal/server, а тесты internal/server уже импортируют
// internal/cli (для подготовки фикстур через cli.Set) — получился бы цикл.
// Тот же выбор направления зависимостей объяснён в internal/index/index.go.
func serve(ctx context.Context, vaultDir, addr, agent, token string) error {
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

	srv := server.New(ix, vaultDir, server.Options{Agent: agent, Token: token})
	httpSrv := &http.Server{Handler: srv.Handler()}

	// Токен печатается один раз, здесь и только здесь: другого способа его
	// узнать нет (в логи и в файлы vault он не пишется), а ссылка с
	// ?token= позволяет открыть доску с телефона одним касанием.
	for _, host := range reachableHosts(ln.Addr()) {
		fmt.Printf("tt serve: http://%s\n", host)
	}
	fmt.Printf("токен на запись (нужен только не с этого компьютера): %s\n", token)
	for _, host := range reachableHosts(ln.Addr()) {
		fmt.Printf("  с телефона: http://%s/?token=%s\n", host, token)
	}

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

// reachableHosts превращает адрес слушателя в список того, что человек может
// набрать в браузере. При привязке к 0.0.0.0 или [::] стандартный Addr() даёт
// «http://[::]:4179» — такое в телефон не введёшь, поэтому для wildcard-адреса
// подставляются реальные адреса машины в сети.
func reachableHosts(addr net.Addr) []string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return []string{addr.String()}
	}
	ip := net.ParseIP(host)
	if ip != nil && !ip.IsUnspecified() {
		return []string{net.JoinHostPort(host, port)}
	}

	// Слушаем на всех интерфейсах — показываем localhost и адреса в сети.
	out := []string{net.JoinHostPort("localhost", port)}
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range ifaces {
		n, ok := a.(*net.IPNet)
		// Отбрасываем link-local (169.254.x.x): это APIPA, адрес «DHCP не ответил»,
		// по нему всё равно ничего не откроется, а в списке онтолько мешает.
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.To4() == nil {
			continue
		}
		out = append(out, net.JoinHostPort(n.IP.String(), port))
	}
	return out
}
