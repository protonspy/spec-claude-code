// testreport runs the suite and prints the one object `scc check` asks a project
// for in its test gate:
//
//	{"total": 412, "coverage": 88.4}
//
// It lives in a nested module so the root's `./...` never sees it. A helper
// package inside the main module would be compiled by `go build ./...`, matched by
// `go test ./...`, and land in the coverage profile at 0% — dragging down the very
// number this program exists to report.
//
// Three properties are the whole contract, and each has a failure mode that looks
// like success:
//
//   - the report goes to stdout and the suite's own output to stderr, so a caller
//     parsing stdout never has to pick the object out of a test log;
//   - the exit status is the suite's, because a runner that swallows a failure
//     hides the thing the gate exists to catch;
//   - total is a real count of tests that ran, not a constant — zero tests at high
//     coverage is the shape of a repository whose tests were deleted.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// module is the root module this reports on. Used to find the repository root
// rather than assuming a working directory: `go run -C tools ./testreport` leaves
// the process in tools/, and a plain `go run ./testreport` from inside tools/ is
// the same situation, but neither is guaranteed by the caller.
const module = "github.com/protonspy/spec-claude-code"

// main does nothing but exit, so that everything below it can use defer. os.Exit
// skips deferred calls, and the one deferred call here removes the coverage
// profile — inside main that would leave a file behind on every run.
func main() { os.Exit(run()) }

func run() int {
	root, err := findRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "testreport:", err)
		return 1
	}

	// A created-exclusive temp file rather than a fixed name under os.TempDir().
	// `go test -coverprofile` opens the path it is handed with create-and-truncate
	// semantics and follows a symlink, so a predictable name on a machine with a
	// shared temp directory lets a co-resident user pre-plant a link and have this
	// process — which the pre-push hook runs — write through it into a file the
	// caller owns.
	f, err := os.CreateTemp("", "scc-testreport-coverage-*.out")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testreport:", err)
		return 1
	}
	profile := f.Name()
	f.Close()
	defer os.Remove(profile)

	total, status, err := runSuite(root, profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testreport:", err)
		return 1
	}

	// Coverage is reported even when the suite failed: a red run still says how
	// much of the tree it reached, and the exit status below is what the gate
	// judges.
	coverage := coverageTotal(root, profile)

	fmt.Printf("{\"total\": %d, \"coverage\": %s}\n", total, coverage)
	return status
}

// findRoot walks up from the working directory for the go.mod that declares the
// root module.
func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(b), "module "+module+"\n") {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod declaring %s above the working directory", module)
		}
		dir = parent
	}
}

// runSuite runs `go test -json` over the whole module, forwards what the tests
// printed to stderr, and returns how many tests passed alongside the suite's exit
// status.
//
// -json rather than parsing the human output because the count has to be a count:
// a pass event carries a Test name, a package-level pass event does not, and only
// the machine stream tells them apart.
func runSuite(root, profile string) (total, status int, err error) {
	cmd := exec.Command("go", "test", "-json", "-coverprofile="+profile, "./...")
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, 0, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, 0, err
	}

	type event struct {
		Action string `json:"Action"`
		Test   string `json:"Test"`
		Output string `json:"Output"`
	}
	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scan.Scan() {
		var e event
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			// Not an event line — `go test` writes build errors as plain text.
			// Pass it through rather than dropping it.
			fmt.Fprintln(os.Stderr, scan.Text())
			continue
		}
		switch e.Action {
		case "pass":
			if e.Test != "" {
				total++
			}
		case "output":
			// The suite's own writing, to stderr: stdout carries the report.
			fmt.Fprint(os.Stderr, e.Output)
		}
	}

	// A scanner that stopped early is not EOF, and treating it as one is the worst
	// failure this program has: a single -json line past the buffer cap ends the
	// loop silently, `total` undercounts with nothing said, and `go test` is left
	// blocked writing into a pipe nobody reads — so the gate hangs rather than
	// going red. Drain first, so the child can always exit, then report it as
	// *could not run* rather than as a result.
	scanErr := scan.Err()
	if scanErr != nil {
		_, _ = io.Copy(io.Discard, stdout)
	}

	// A non-zero exit is the suite failing, which is a result rather than an error
	// here — it is what the gate is asking about.
	waitErr := cmd.Wait()
	if scanErr != nil {
		return total, 0, fmt.Errorf("reading `go test -json` output: %w", scanErr)
	}
	if waitErr != nil {
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) {
			return total, 0, waitErr
		}
		return total, exit.ExitCode(), nil
	}
	return total, 0, nil
}

// coverageTotal reads the percentage off `go tool cover -func`'s total line.
// A profile that does not exist or a tool that fails yields "0" rather than an
// error: the suite's status is the answer the gate wants, and a missing coverage
// number below any floor is the honest report.
func coverageTotal(root, profile string) string {
	if _, err := os.Stat(profile); err != nil {
		return "0"
	}
	cmd := exec.Command("go", "tool", "cover", "-func="+profile)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "0"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "total:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			break
		}
		return strings.TrimSuffix(fields[len(fields)-1], "%")
	}
	return "0"
}
