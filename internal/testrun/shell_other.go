//go:build !windows

package testrun

import (
	"context"
	"os/exec"
)

// command builds the process that runs a recorded command line: `sh -c`, which
// is what every other tool that takes a command string does and what the person
// who wrote the string was thinking of.
func command(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}
