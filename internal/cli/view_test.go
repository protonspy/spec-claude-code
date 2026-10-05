package cli

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
)

// stopAtOnce swaps the interrupt for a context that is already cancelled, so
// runView binds, prints its URL, and returns the way an interrupt makes it return.
func stopAtOnce(t *testing.T) {
	t.Helper()
	orig := viewContext
	viewContext = func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx, cancel
	}
	t.Cleanup(func() { viewContext = orig })
}

func TestViewPrintsALoopbackURLAndExitsZeroOnInterrupt(t *testing.T) {
	stopAtOnce(t)
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "view", "--port", "0", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	var got struct{ URL, Root string }
	decode(t, stdout, &got)
	if !strings.HasPrefix(got.URL, "http://127.0.0.1:") || strings.HasSuffix(got.URL, ":0/") {
		t.Errorf("url = %q, want a bound 127.0.0.1 port", got.URL)
	}
}

func TestViewRefusesOutsideAWorkspace(t *testing.T) {
	stopAtOnce(t)
	_, stderr, code := run(t, "view", "--port", "0", "--root", t.TempDir())
	if code != ExitError || !strings.Contains(stderr, "not an scc workspace") {
		t.Errorf("exit = %d, stderr %q; want 1 naming the missing workspace", code, stderr)
	}
}

func TestViewReportsAPortInUse(t *testing.T) {
	stopAtOnce(t)
	root := initWorkspace(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	_, stderr, code := run(t, "view", "--port", port, "--root", root)
	if code != ExitError || !strings.Contains(stderr, "127.0.0.1:"+port) {
		t.Errorf("exit = %d, stderr %q; want 1 naming 127.0.0.1:%s", code, stderr, port)
	}
}

func TestViewRejectsABadPort(t *testing.T) {
	for _, p := range []string{"-1", "65536"} {
		if _, _, code := run(t, "view", "--port", p); code != ExitError {
			t.Errorf("--port %s exit = %d, want 1", p, code)
		}
	}
}

func TestViewHelpIsZero(t *testing.T) {
	for _, args := range [][]string{{"view", "--help"}, {"view", "help"}} {
		if _, stderr, code := run(t, args...); code != ExitOK || !strings.Contains(stderr, "view [--port N]") {
			t.Errorf("%v exit = %d, stderr %q", args, code, stderr)
		}
	}
}
