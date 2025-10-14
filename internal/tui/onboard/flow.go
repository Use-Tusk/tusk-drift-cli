package onboard

type Step interface {
	ID() onboardStep
	InputIndex() int               // Index of the input field (0-based) for this step, -1 if no input is required
	Question(*Model) string        // Prompt for the user
	Description(*Model) string     // Additional information displayed below the question
	Default(*Model) string         // Default input value (empty string if not applicable)
	Validate(*Model, string) error // User input validation (returns nil if valid, error if invalid)
	Apply(*Model, string)          // Updates the model with the accepted user input. Called after validation succeeds.
	Help(*Model) string            // Help text displayed in the footer for the current step. Back navigation keys will be dynamically added.
	ShouldSkip(*Model) bool        // Whether the step should be skipped (based on current state)
	Clear(*Model)                  // Clears the current step's state
}

type BaseStep struct{}

func (BaseStep) Validate(*Model, string) error { return nil }
func (BaseStep) Apply(*Model, string)          {}
func (BaseStep) Help(*Model) string            { return "enter: use default • esc: quit" }
func (BaseStep) ShouldSkip(*Model) bool        { return false }
func (BaseStep) Clear(*Model)                  {}

type Flow struct {
	steps []Step
	index map[onboardStep]int
}

func NewFlow(steps []Step) *Flow {
	idx := make(map[onboardStep]int, len(steps))
	for i, s := range steps {
		idx[s.ID()] = i
	}
	return &Flow{steps: steps, index: idx}
}

func (f *Flow) Current(i int) Step { return f.steps[i] }

func (f *Flow) NextIndex(i int, m *Model) int {
	j := i + 1
	for j < len(f.steps) && f.steps[j].ShouldSkip(m) {
		j++
	}
	if j >= len(f.steps) {
		return i
	}
	return j
}

func (f *Flow) PrevIndex(history []int) (int, []int, bool) {
	if len(history) == 0 {
		return 0, history, false
	}
	n := history[len(history)-1]
	return n, history[:len(history)-1], true
}
