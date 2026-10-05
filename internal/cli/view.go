package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"

	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/view"
)

// defaultViewPort is fixed rather than 0 so the URL survives a restart and a tab
// left open reconnects; --port 0 asks the system for a free one instead.
const defaultViewPort = 7777

// viewContext is what stops the server: an interrupt, in the real binary. A test
// swaps it for a context it cancels itself, since it cannot signal its own process.
var viewContext = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// runView serves the workspace read-only on loopback until interrupted. It is the
// one command that does not return on its own, and an interrupt is its normal end,
// so that exits 0.
func runView(args []string) int {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = viewUsage
	root := addRoot(fs)
	port := fs.Int("port", defaultViewPort, "port to listen on, on 127.0.0.1 only; 0 picks a free one")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, helpWord(args))
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "view") {
		return ExitError
	}
	if *port < 0 || *port > 65535 {
		render.Err(fmt.Sprintf("--port %d is not a port", *port))
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(*port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		render.Err(fmt.Sprintf("could not listen on %s: %v", addr, err))
		render.Detail("  another process holds the port — pass --port <n>, or --port 0 for a free one")
		return ExitError
	}
	url := "http://" + ln.Addr().String() + "/"

	ctx, stop := viewContext()
	defer stop()
	if *jsonOut {
		if code := emitJSON(struct {
			URL  string `json:"url"`
			Root string `json:"root"`
		}{url, target}); code != ExitOK {
			_ = ln.Close()
			return code
		}
	} else {
		render.OK("serving " + target)
		render.Info(url)
		render.Detail("  read-only, loopback only — Ctrl+C to stop")
	}
	if err := view.Serve(ctx, ln, target); err != nil {
		render.Err(fmt.Sprintf("server stopped: %v", err))
		return ExitError
	}
	return ExitOK
}

func viewUsage() {
	fmt.Fprintf(os.Stderr, `Usage: %s view [--port N] [--root DIR] [--json]

Serve the workspace's docs, plans and specs as a read-only web page on
127.0.0.1, until interrupted: the wiki, ADRs, codewiki with the source lines it
cites, glossary, stack, notes, every plan and every spec.

Flags:
  --port N    port to listen on (default %d; 0 picks a free one)
  --root DIR  %s
  --json      print {"url", "root"} on stdout once listening
`, prog(), defaultViewPort, rootFlagHelp)
}
