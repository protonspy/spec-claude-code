package artifact

import "sort"

// Which task comes next, decided once.
//
// `--next`, `--ready` and `--blocked` are three views of one question, so they share
// one implementation. Two notions of eligibility would be two answers to "what do I
// work on", and the loop would take whichever it asked first.

// resolveTaskStates fills in the derived half of every task: whether its
// dependencies are satisfied, and therefore whether it can be started.
//
// A task that is done or removed is neither eligible nor blocked — it is finished
// with, and reporting it as blocked would put it in the impasse listing forever.
func (a *Artifact) resolveTaskStates() {
	byNumber := make(map[string]int, len(a.Tasks))
	for i := range a.Tasks {
		if _, seen := byNumber[a.Tasks[i].Number]; !seen {
			byNumber[a.Tasks[i].Number] = i
		}
	}
	for i := range a.Tasks {
		t := &a.Tasks[i]
		t.Eligible, t.Blocked = false, false
		if t.Checked || t.Removed() {
			continue
		}
		ready := true
		for _, d := range t.Depends {
			j, found := byNumber[d]
			if !found || !a.Tasks[j].Checked {
				ready = false
				break
			}
		}
		t.Eligible, t.Blocked = ready, !ready
	}
}

// WaitingOn is what a blocked task is waiting for: the dependencies that are not
// done, plus the ones that do not exist. A blocked task that could not name its
// blocker would be an impasse with no way out.
func (a *Artifact) WaitingOn(t Task) []string {
	var out []string
	for _, d := range t.Depends {
		dep, ok := a.Task(d)
		if !ok || !dep.Checked {
			out = append(out, d)
		}
	}
	return out
}

// Ready is the eligible tasks in the order a loop should take them: by priority,
// then by number read as numbers, then by position in the file.
//
// Priority is ascending and absent sorts last, which is the reading a person
// expects — "priority 1" is the urgent one, and a task nobody prioritized is not
// more urgent than one somebody did.
func (a *Artifact) Ready() []Task {
	var out []Task
	for _, t := range a.Tasks {
		if t.Eligible {
			out = append(out, t)
		}
	}
	SortTasks(out)
	return out
}

// BlockedTasks is the open tasks that are not eligible, in the same order.
func (a *Artifact) BlockedTasks() []Task {
	var out []Task
	for _, t := range a.Tasks {
		if t.Blocked {
			out = append(out, t)
		}
	}
	SortTasks(out)
	return out
}

// Next is the task to work on, if there is one.
func (a *Artifact) Next() (Task, bool) {
	ready := a.Ready()
	if len(ready) == 0 {
		return Task{}, false
	}
	return ready[0], true
}

// SortTasks orders tasks the way §"what do I work on next" defines: priority
// ascending with absent last, then number numerically, then file order as a total
// tie-break so the result never depends on the sort's stability.
func SortTasks(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		pa, pb := priorityOf(a), priorityOf(b)
		if pa != pb {
			return pa < pb
		}
		if c := CompareNumbers(a.Number, b.Number); c != 0 {
			return c < 0
		}
		return a.Line < b.Line
	})
}

// priorityOf is the task's priority, or a value past every real one.
const noPriority = int(^uint(0) >> 1)

func priorityOf(t Task) int {
	if t.Priority == nil {
		return noPriority
	}
	return *t.Priority
}

// DependencyCycles returns every cycle in the task graph, each as the numbers on it
// starting from its smallest member so the same cycle reports identically twice.
//
// A cycle is reported rather than worked around because there is no right answer to
// work around it with: every task on it is waiting for another one on it, and a
// `--next` that picked one anyway would start work whose dependency will never be
// satisfied.
func (a *Artifact) DependencyCycles() [][]string { return Cycles(a.Tasks) }

// Cycles is DependencyCycles over a bare task list, so the validator can report one
// without building an Artifact around the document it is already holding.
func Cycles(tasks []Task) [][]string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	index := make(map[string]Task, len(tasks))
	var order []string
	for _, t := range tasks {
		if t.Number == "" {
			continue
		}
		if _, seen := index[t.Number]; seen {
			continue
		}
		index[t.Number] = t
		order = append(order, t.Number)
	}

	seen := map[string]bool{}
	var cycles [][]string
	var stack []string

	var visit func(string)
	visit = func(n string) {
		color[n] = grey
		stack = append(stack, n)
		for _, d := range index[n].Depends {
			if _, ok := index[d]; !ok {
				continue
			}
			switch color[d] {
			case white:
				visit(d)
			case grey:
				at := len(stack) - 1
				for at >= 0 && stack[at] != d {
					at--
				}
				if at < 0 {
					continue
				}
				cycle := append([]string(nil), stack[at:]...)
				if key := cycleKey(cycle); !seen[key] {
					seen[key] = true
					cycles = append(cycles, normalizeCycle(cycle))
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
	}
	for _, n := range order {
		if color[n] == white {
			visit(n)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return CompareNumbers(cycles[i][0], cycles[j][0]) < 0 })
	return cycles
}

// normalizeCycle rotates a cycle so it starts at its smallest number, which is what
// makes the same loop report the same way whichever node the walk entered it from.
func normalizeCycle(cycle []string) []string {
	at := 0
	for i, n := range cycle {
		if CompareNumbers(n, cycle[at]) < 0 {
			at = i
		}
	}
	return append(append([]string(nil), cycle[at:]...), cycle[:at]...)
}

func cycleKey(cycle []string) string {
	sorted := append([]string(nil), cycle...)
	sort.Strings(sorted)
	key := ""
	for _, n := range sorted {
		key += n + "\x00"
	}
	return key
}
