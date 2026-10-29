package runner

import "context"

// Fake records the commands it receives and returns whatever Handler returns.
// A nil Handler yields an empty result and no error.
type Fake struct {
	Handler func(Command) (string, error)
	Calls   []Command
}

// Run implements Runner.
func (f *Fake) Run(_ context.Context, c Command) (string, error) {
	f.Calls = append(f.Calls, c)
	if f.Handler == nil {
		return "", nil
	}
	return f.Handler(c)
}
