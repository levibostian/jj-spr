// Package runner runs external commands behind a small interface so callers
// can be tested with a fake instead of a real jj/gh binary.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Command is one external process invocation.
type Command struct {
	Name  string
	Args  []string
	Stdin string
}

// String renders the command as it would be typed in a shell.
func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Runner runs an external command and returns its stdout.
type Runner interface {
	Run(ctx context.Context, cmd Command) (string, error)
}

// Exec runs commands with os/exec.
type Exec struct{}

// Run implements Runner.
func (Exec) Run(ctx context.Context, c Command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s: %s", c.String(), msg)
	}
	return stdout.String(), nil
}

// Verbose logs each command to the writer before running it.
type Verbose struct {
	Inner  Runner
	Writer io.Writer
}

// Run implements Runner.
func (v Verbose) Run(ctx context.Context, c Command) (string, error) {
	fmt.Fprintln(v.Writer, "+", c.String())
	return v.Inner.Run(ctx, c)
}
