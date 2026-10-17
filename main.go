// Command memtree is persistent memory for coding agents: a local service with
// an MCP server, harness hooks and a desktop (or browser) UI to inspect and
// edit what agents remember.
//
//	memtree                          launch the desktop app
//	memtree serve                    run headless and open the UI in a browser
//	memtree mcp                      stdio MCP bridge to the running app (Claude Desktop & other stdio clients)
//	memtree install <harness>        wire hooks + MCP into claude-code, codex, cursor or vscode
//	memtree hook <harness> [event]   run a hook (called by the harness, not by you)
//	memtree recall <query>           search memory from the terminal
//	memtree version
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/aitakemyjob/memtree/internal/api"
	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/core"
	"github.com/aitakemyjob/memtree/internal/hooks"
	"github.com/aitakemyjob/memtree/internal/server"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `memtree %s — persistent memory for coding agents.

Usage:
  memtree [flags]                       launch the desktop app
  memtree serve [flags]                 run without a window and use the UI in your browser
  memtree mcp [flags]                   stdio MCP bridge to the running app (falls back to standalone)
  memtree install <harness> [flags]     wire hooks and MCP into a harness
  memtree hook <harness> [event]        run a harness hook (reads the payload on stdin)
  memtree recall [flags] <query>        search memory from the terminal
  memtree version                       print the version

Harnesses: claude-code, codex, cursor, vscode

Flags:
  -port N        loopback port (default from settings, %d)
  -open          (serve) open the browser automatically (default true)
  -standalone    (mcp) serve stdio directly instead of bridging to the app
  -project DIR   (install, recall) scope to a project directory; install: write project config instead of user config
  -dry-run       (install) show what would change without writing
  -uninstall     (install) remove memtree from the harness
  -all           (recall) search every project

The MCP endpoint is http://127.0.0.1:<port>/mcp (streamable HTTP).
`

type app struct {
	core   *core.Service
	api    *api.API
	server *server.Server
	hooks  *hooks.Manager
	cancel context.CancelFunc
}

// bootstrap loads config, opens the database and wires the service, API,
// hooks manager and HTTP server. withUI also serves the embedded frontend.
func bootstrap(port int, withUI bool) (*app, error) {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Open(cfgPath)
	if err != nil {
		return nil, err
	}
	svc, err := core.New(cfg, version)
	if err != nil {
		return nil, err
	}
	if port == 0 {
		port = cfg.Get().MCPPort
	}
	a := api.New(svc)
	mgr := hooks.NewManager(svc)
	a.Hooks = mgr
	opts := server.Options{Port: port}
	if withUI {
		ui, err := fs.Sub(assets, "frontend/dist")
		if err != nil {
			return nil, err
		}
		opts.UI = ui
	}
	ctx, cancel := context.WithCancel(context.Background())
	go svc.RunMaintenance(ctx)
	return &app{core: svc, api: a, server: server.New(svc, a, opts), hooks: mgr, cancel: cancel}, nil
}

// close stops background maintenance and closes the database.
func (a *app) close() {
	a.cancel()
	a.core.Close()
}

// main dispatches the subcommands.
func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	fl := flag.NewFlagSet("memtree", flag.ExitOnError)
	port := fl.Int("port", 0, "loopback port")
	open := fl.Bool("open", true, "open the browser (serve)")
	standalone := fl.Bool("standalone", false, "(mcp) serve stdio directly")
	project := fl.String("project", "", "project directory")
	dry := fl.Bool("dry-run", false, "(install) report only")
	uninstall := fl.Bool("uninstall", false, "(install) remove")
	all := fl.Bool("all", false, "(recall) all projects")
	fl.Usage = func() { fmt.Fprintf(os.Stderr, usage, version, config.DefaultMCPPort) }

	// hook payloads and subcommand positionals come before/after flags in
	// varying order; parse flags first, then take what is left.
	if cmd == "hook" {
		os.Exit(runHook(args))
	}
	rest := parseInterspersed(fl, args)

	var err error
	switch cmd {
	case "", "gui":
		err = runGUI(*port)
	case "serve":
		err = runServe(*port, *open)
	case "mcp":
		err = runMCP(*port, *standalone)
	case "install":
		err = runInstall(rest, *project, *dry, *uninstall)
	case "recall":
		err = runRecall(strings.Join(rest, " "), *project, *all)
	case "version", "--version":
		fmt.Println("memtree", version)
	case "help":
		fl.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		fl.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "memtree:", err)
		os.Exit(1)
	}
}

// runServe runs headless, serving the UI and MCP over HTTP until interrupted.
func runServe(port int, open bool) error {
	a, err := bootstrap(port, true)
	if err != nil {
		return err
	}
	defer a.close()
	if err := a.server.Start(); err != nil {
		return err
	}
	url := a.server.URL()
	fmt.Printf("memtree %s\n  UI:  %s\n  MCP: %s/mcp\n", version, url, url)
	if open {
		openBrowser(url)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.server.Shutdown(shutdown)
}

// runInstall wires (or unwires) a harness.
func runInstall(rest []string, project string, dry, uninstall bool) error {
	if len(rest) != 1 {
		return fmt.Errorf("usage: memtree install <%s> [-project DIR] [-dry-run] [-uninstall]", strings.Join(hooks.Harnesses, "|"))
	}
	a, err := bootstrap(0, false)
	if err != nil {
		return err
	}
	defer a.close()
	if project != "" {
		project = core.ResolveProject(project)
	}
	var res any
	if uninstall {
		res, err = a.hooks.Uninstall(context.Background(), rest[0], project)
	} else {
		res, err = a.hooks.Install(context.Background(), rest[0], project, dry)
	}
	if r, ok := res.(*hooks.Result); ok && r != nil {
		verb := "Changes"
		if r.DryRun {
			verb = "Would change (dry run)"
		}
		fmt.Println(verb + ":")
		for _, c := range r.Changes {
			fmt.Printf("  %-6s %s — %s\n", c.Action, c.File, c.Detail)
		}
		if len(r.Changes) == 0 {
			fmt.Println("  nothing")
		}
		for _, n := range r.Notes {
			fmt.Println("\n" + n)
		}
	}
	return err
}

// runRecall is a terminal front end to recall, handy for checking what an
// agent would see.
func runRecall(query, project string, all bool) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("usage: memtree recall [-project DIR] [-all] <query>")
	}
	a, err := bootstrap(0, false)
	if err != nil {
		return err
	}
	defer a.close()
	if project == "" {
		project, _ = os.Getwd()
	}
	res, err := a.core.Recall(context.Background(), core.RecallInput{Query: query, Project: core.ResolveProject(project), AllProjects: all, Limit: 8})
	if err != nil {
		return err
	}
	if len(res.Memories) == 0 {
		fmt.Println("No matching memories.")
		return nil
	}
	now := a.core.DB.Now()
	for _, m := range res.Memories {
		fmt.Printf("%s  %.2f  %s  %s\n    %s\n", m.Record.ID, m.Score.Final, core.Ago(now.Sub(m.Record.CreatedAt)), m.Source, oneLine(m.Record.Summary))
	}
	return nil
}

// oneLine collapses whitespace and shortens s for terminal output.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

// runHook is what a harness invokes. It must never block or fail the agent:
// it always exits 0, prints only what the harness should inject, and logs
// problems to hook-errors.log next to the config file.
func runHook(args []string) int {
	if len(args) < 1 {
		return 0
	}
	harness, event := args[0], ""
	if len(args) > 1 {
		event = args[1]
	}
	payload, _ := readAll(os.Stdin, 4<<20)
	out, err := hookViaService(harness, event, payload)
	if err == errNoService {
		out, err = hookDirect(harness, event, payload)
	}
	if err != nil {
		logHookError(harness, event, err)
	}
	if out != "" {
		fmt.Println(out)
	}
	return 0
}

// openBrowser opens url in the default browser (best effort).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("could not open browser", "err", err)
	}
}

// parseInterspersed parses flags that may come before or after positional
// arguments ("install claude-code -dry-run"), returning the positionals.
// The standard flag package stops at the first positional.
func parseInterspersed(fl *flag.FlagSet, args []string) []string {
	var pos []string
	for {
		fl.Parse(args)
		args = fl.Args()
		if len(args) == 0 {
			return pos
		}
		if args[0] == "--" {
			return append(pos, args[1:]...)
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}
