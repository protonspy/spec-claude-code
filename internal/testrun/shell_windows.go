//go:build windows

package testrun

import (
	"context"
	"os/exec"
	"syscall"
)

// command builds the process that runs a recorded command line on Windows.
//
// The command line is handed to cmd.exe verbatim, through SysProcAttr, rather
// than as an argument vector. That is not a style choice: os/exec quotes each
// argument for the C runtime's rules, and cmd.exe does not use those rules — a
// recorded `go test -run "TestX"` arrives at cmd as `-run \"TestX\"` and runs a
// test nobody named. Setting the line ourselves makes scc run exactly what the
// user would have typed, which is the only contract a recorded command can have.
func command(ctx context.Context, line string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd /c " + line}
	return cmd
}
