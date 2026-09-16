package cli

import (
	"strings"
	"testing"
)

// rtkInstallOK is one decision for `scc init` and `scc launch` both, because two
// copies would be two answers to "is anybody here to ask" and the untested one
// would be the one that ran. Every path out of it is a reason phrased for the line
// the caller prints.
func TestRTKInstallOKAnswersEveryWayItCanRefuse(t *testing.T) {
	// No cargo anywhere: the toolchain is the reason, not the flag.
	isolatedPath(t)

	for name, tc := range map[string]struct {
		ask  rtkAsk
		says string
	}{
		"no-install":  {rtkAsk{noInstall: true}, "not on PATH"},
		"plan only":   {rtkAsk{plan: true}, "not on PATH"},
		"no cargo":    {rtkAsk{}, "cargo is not on PATH"},
		"unattended":  {rtkAsk{quiet: true}, "cargo is not on PATH"},
		"yes without": {rtkAsk{yes: true}, "cargo is not on PATH"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _ = capture(t, func() int {
				if got := rtkInstallOK(tc.ask); !strings.Contains(got, tc.says) {
					t.Errorf("rtkInstallOK(%+v) = %q, want it to mention %q", tc.ask, got, tc.says)
				}
				return ExitOK
			})
		})
	}

	// With cargo there, --yes is consent in advance and nothing is asked.
	isolatedPath(t, "cargo")
	if got := rtkInstallOK(rtkAsk{yes: true}); got != "" {
		t.Errorf("rtkInstallOK with --yes and cargo present = %q, want the empty answer", got)
	}
	// And an unattended run still declines rather than building a Rust binary on
	// somebody who is not there to stop it.
	if got := rtkInstallOK(rtkAsk{quiet: true}); !strings.Contains(got, "nobody is here") {
		t.Errorf("an unattended run = %q, want it to say nobody is here to answer", got)
	}
	// --no-install outranks a present cargo: it is the flag that means "install
	// nothing", not "install nothing unless it is easy".
	if got := rtkInstallOK(rtkAsk{noInstall: true}); got == "" {
		t.Error("--no-install with cargo present allowed the build anyway")
	}
}
