package runner

import (
	"context"
	"strings"
	"testing"
)

func TestCommandString(t *testing.T) {
	got := Command{Name: "gh", Args: []string{"pr", "view", "1"}}.String()
	if got != "gh pr view 1" {
		t.Fatalf("got %q", got)
	}
}

func TestExecRun(t *testing.T) {
	out, err := Exec{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "printf hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestExecStdin(t *testing.T) {
	out, err := Exec{}.Run(context.Background(), Command{
		Name:  "sh",
		Args:  []string{"-c", "cat"},
		Stdin: "body text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "body text" {
		t.Fatalf("got %q", out)
	}
}

func TestExecEnv(t *testing.T) {
	out, err := Exec{}.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", `printf %s "$JJSPR_TEST"`},
		Env:  []string{"JJSPR_TEST=hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestExecErrorIncludesStderr(t *testing.T) {
	_, err := Exec{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "echo boom >&2; exit 3"}})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want stderr in error, got %v", err)
	}
}

func TestFakeRecordsAndReturns(t *testing.T) {
	f := &Fake{Handler: func(c Command) (string, error) { return "out:" + c.Name, nil }}
	got, err := f.Run(context.Background(), Command{Name: "gh"})
	if err != nil || got != "out:gh" {
		t.Fatalf("got %q %v", got, err)
	}
	if len(f.Calls) != 1 || f.Calls[0].Name != "gh" {
		t.Fatalf("calls not recorded: %+v", f.Calls)
	}
}

func TestVerboseLogs(t *testing.T) {
	var sb strings.Builder
	v := Verbose{Inner: &Fake{}, Writer: &sb}
	if _, err := v.Run(context.Background(), Command{Name: "jj", Args: []string{"log"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "jj log") {
		t.Fatalf("got %q", sb.String())
	}
}
