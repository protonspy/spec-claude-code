package artifact

import "strconv"

// The delivery record: three frontmatter keys that say where a spec is being built
// and how far that has got.
//
// It exists because a branch is the one part of this methodology that leaves no trace
// in the artifacts. A spec says what the feature does and which tasks are ticked; git
// says a branch called `feat/user-auth` has been sitting unmerged for three weeks.
// Nothing joined the two, so "which specs are actually finished" was a question only a
// person holding both halves in their head could answer — and under `autonomy: auto`
// there is no such person. These keys are the join, and `scc spec sync` is what keeps
// them true without anybody remembering to.
//
// They live on `requirements.md`, with the kickoff answers, because that is already
// the spec's header: one file to read for everything about the spec that is not a
// requirement, a design, or a task.
const (
	// KeyBranch is the branch the work is on, as git spells it.
	KeyBranch = "branch"
	// KeyPR is the pull request's number, digits only. A number rather than a URL
	// because it is what `gh pr view` takes and what a person says out loud; the URL
	// is reconstructible and the number is not.
	KeyPR = "pr"
	// KeyDelivery is how far the work has got, from DeliveryStates.
	//
	// Deliberately not `status:`, which already means the approval seal on a plan
	// (see KeyStatus). One key with two meanings across two artifacts is the defect
	// this vocabulary exists to avoid, and `delivery:` names its own concern —
	// the rule that owns it is delivery.md.
	KeyDelivery = "delivery"
)

// The delivery states, and the whole set of them.
//
// Four, and the set is closed for the same reason a task's flags are: an open
// vocabulary here would produce three spellings of "done" inside a month, and the
// question this record answers — what is still unfinished — is exactly the one a
// synonym destroys.
//
// Absent is the fifth state and needs no name: a spec nobody has started carries no
// branch, no PR, and no delivery line, which is also what every spec written before
// this shipped looks like.
const (
	// DeliveryInProgress — a branch exists and has not landed.
	DeliveryInProgress = "in-progress"
	// DeliveryInReview — a pull request is open.
	DeliveryInReview = "in-review"
	// DeliveryMerged — the work is on the base branch. Terminal, and the reason the
	// branch and PR are kept rather than cleared: the spec then permanently records
	// what delivered it.
	DeliveryMerged = "merged"
	// DeliveryAbandoned — the pull request was closed unmerged, or the branch was
	// dropped. Terminal, and it has to be sayable: a spec that was tried and dropped
	// is not the same as one nobody started, and only one of the two is a loose end.
	DeliveryAbandoned = "abandoned"
)

// DeliveryStates returns the vocabulary, in the order work moves through it.
func DeliveryStates() []string {
	return []string{DeliveryInProgress, DeliveryInReview, DeliveryMerged, DeliveryAbandoned}
}

// ValidDelivery reports whether s is one of them.
func ValidDelivery(s string) bool {
	for _, v := range DeliveryStates() {
		if s == v {
			return true
		}
	}
	return false
}

// Settled reports whether a state is terminal — the work is not coming back. It is
// what separates "still open" from "done with", which is the only distinction a
// reader scanning for loose ends actually makes.
func Settled(state string) bool {
	return state == DeliveryMerged || state == DeliveryAbandoned
}

// Delivery is the record as one value.
type Delivery struct {
	Branch string `json:"branch,omitempty"`
	// PR is 0 when there is none. A pointer would distinguish "absent" from "zero",
	// and there is no pull request number zero, so it would only buy a nil check at
	// every use.
	PR    int    `json:"pr,omitempty"`
	State string `json:"delivery,omitempty"`
}

// Tracked reports whether anything has been recorded at all. An untracked spec is not
// a defect — it is a spec nobody has started, or one that predates the record.
func (d Delivery) Tracked() bool { return d.Branch != "" || d.PR != 0 || d.State != "" }

// ReadDelivery pulls the record out of a parsed frontmatter map.
//
// A `pr:` that is not a number comes back as 0 rather than as an error: reading is
// not where a malformed value gets reported, the validator is, and a reader that
// failed here would take out `scc spec list` for the whole workspace over one typo.
func ReadDelivery(fm map[string]string) Delivery {
	d := Delivery{Branch: fm[KeyBranch], State: fm[KeyDelivery]}
	if n, err := strconv.Atoi(fm[KeyPR]); err == nil && n > 0 {
		d.PR = n
	}
	return d
}
