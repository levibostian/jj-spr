package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
)

func jjLine(changeID, desc string) string {
	return fmt.Sprintf(`{"commit_id":"sha-%s","change_id":%q,"description":%q}`, changeID, changeID, desc) +
		"\x1f" + strings.Join([]string{"0", changeID, changeID}, "\x1f") + "\n"
}

// fullHandler scripts a two-commit stack with no existing PRs.
func fullHandler(linked *bool, creates *int, existing bool) func(runner.Command) (string, error) {
	return func(c runner.Command) (string, error) {
		s := c.String()
		switch {
		case strings.Contains(s, "gh stack --help"):
			return "", nil
		case strings.Contains(s, "remote_bookmarks"):
			return "main\n", nil
		case strings.Contains(s, "git remote list"):
			return "origin git@github.com:me/repo.git\n", nil
		case strings.Contains(s, "bookmark list"):
			return "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\n", nil
		case strings.Contains(s, "~::"):
			return "C2\x1fmystack\n", nil
		case strings.Contains(s, "conflicts()"):
			return "", nil
		case strings.Contains(s, "jj log"):
			return jjLine("C1", "Add model\n") + jjLine("C2", "Add API\n"), nil
		case strings.Contains(s, "pulls"):
			if existing {
				return `[{"number":1,"state":"open","title":"Add model","body":"","html_url":"u","head":{"ref":"mystack-C1"},"base":{"ref":"main"}}]`, nil
			}
			return "[]", nil
		case strings.Contains(s, "pr create"):
			*creates++
			return fmt.Sprintf("https://github.com/me/repo/pull/%d\n", *creates), nil
		case strings.Contains(s, "pr view"):
			return `{"number":1,"title":"Add model","state":"OPEN","mergeable":"MERGEABLE","reviewDecision":"APPROVED","statusCheckRollup":[{"state":"SUCCESS"}]}`, nil
		case strings.Contains(s, "stack link"):
			*linked = true
			return "", nil
		case strings.Contains(s, "stacks?pull_request"):
			if *linked {
				return `[{"number":9,"pull_requests":[{"number":1,"state":"open"},{"number":2,"state":"open"}]}]`, nil
			}
			return "[]", nil
		}
		return "", nil
	}
}

func newTestApp() (*App, *runner.Fake, *bytes.Buffer, *bytes.Buffer) {
	f := &runner.Fake{}
	var out, errb bytes.Buffer
	return &App{Runner: f, Stdout: &out, Stderr: &errb}, f, &out, &errb
}

func TestUsageAndUnknown(t *testing.T) {
	app, _, out, errb := newTestApp()
	if code := app.Run(context.Background(), nil); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(out.String(), "Usage") {
		t.Fatalf("out %q", out.String())
	}
	if code := app.Run(context.Background(), []string{"bogus"}); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("err %q", errb.String())
	}
}

func TestHelpAndVersion(t *testing.T) {
	app, _, out, _ := newTestApp()
	if code := app.Run(context.Background(), []string{"--help"}); code != 0 {
		t.Fatal(code)
	}
	if code := app.Run(context.Background(), []string{"version"}); code != 0 || !strings.Contains(out.String(), "0.1.0") {
		t.Fatalf("version: %q", out.String())
	}
}

func TestBadFlag(t *testing.T) {
	app, _, _, _ := newTestApp()
	if code := app.Run(context.Background(), []string{"update", "--nope"}); code != 2 {
		t.Fatal(code)
	}
	if code := app.Run(context.Background(), []string{"status", "extra"}); code != 2 {
		t.Fatal(code)
	}
}

func TestUpdateWiring(t *testing.T) {
	app, f, out, _ := newTestApp()
	linked, creates := false, 0
	f.Handler = fullHandler(&linked, &creates, false)
	if code := app.Run(context.Background(), []string{"update", "--base", "main"}); code != 0 {
		t.Fatalf("code %d, out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "created:") || !strings.Contains(out.String(), "stack: #9") {
		t.Fatalf("out %q", out.String())
	}
	if !strings.Contains(out.String(), "https://github.com/me/repo/pull/1") ||
		!strings.Contains(out.String(), "https://github.com/me/repo/pull/2") {
		t.Fatalf("want clickable PR URLs, out %q", out.String())
	}
}

func TestUpdateError(t *testing.T) {
	app, f, _, errb := newTestApp()
	f.Handler = func(c runner.Command) (string, error) {
		if strings.Contains(c.String(), "stack --help") {
			return "", nil
		}
		return "", fmt.Errorf("boom")
	}
	if code := app.Run(context.Background(), []string{"update"}); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "error:") {
		t.Fatalf("err %q", errb.String())
	}
}

func TestUpdateMissingExtension(t *testing.T) {
	app, f, _, errb := newTestApp()
	f.Handler = func(c runner.Command) (string, error) {
		switch {
		case strings.Contains(c.String(), "git remote list"):
			return "origin git@github.com:me/repo.git\n", nil
		case strings.Contains(c.String(), "stack --help"):
			return "", fmt.Errorf("no stack")
		}
		return "", nil
	}
	if code := app.Run(context.Background(), []string{"update"}); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "gh extension install") {
		t.Fatalf("err %q", errb.String())
	}
}

func TestStatusWiring(t *testing.T) {
	app, f, out, _ := newTestApp()
	linked, creates := false, 0
	f.Handler = fullHandler(&linked, &creates, true)
	if code := app.Run(context.Background(), []string{"status", "--base", "main"}); code != 0 {
		t.Fatalf("code %d out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "mystack") || !strings.Contains(out.String(), "#1") {
		t.Fatalf("out %q", out.String())
	}
}

// TestStatusOutput pins the exact stdout of `status`: the rows followed by the
// legend, whose connectors line up under the last row's emojis.
func TestStatusOutput(t *testing.T) {
	app, f, out, errb := newTestApp()
	f.Handler = func(c runner.Command) (string, error) {
		s := c.String()
		branch1, branch2, branch3 := "mystack-C1", "mystack-C2", "mystack-C3"
		switch {
		case strings.Contains(s, "git remote list"):
			return "origin git@github.com:me/repo.git\n", nil
		case strings.Contains(s, "remote_bookmarks"):
			return "main\n", nil
		case strings.Contains(s, "bookmark list"):
			return "mystack\tC3\n" + branch1 + "\tC1\n" + branch2 + "\tC2\n" + branch3 + "\tC3\n", nil
		case strings.Contains(s, "~::"):
			return "C3\x1fmystack\n", nil
		case strings.Contains(s, "jj log"):
			return jjLine("C1", "Add model\n") + jjLine("C2", "Add API\n") + jjLine("C3", "Add UI\n"), nil
		case strings.Contains(s, "pulls"):
			switch {
			case strings.Contains(s, branch1):
				return `[{"number":41,"state":"open","head":{"ref":"` + branch1 + `"},"base":{"ref":"main"}}]`, nil
			case strings.Contains(s, branch2):
				return `[{"number":42,"state":"open","head":{"ref":"` + branch2 + `"},"base":{"ref":"main"}}]`, nil
			case strings.Contains(s, branch3):
				return `[{"number":43,"state":"open","head":{"ref":"` + branch3 + `"},"base":{"ref":"main"}}]`, nil
			}
			return "[]", nil
		case strings.Contains(s, "pr view"):
			switch {
			case strings.Contains(s, branch1):
				return `{"number":41,"title":"Add model","state":"OPEN","mergeable":"MERGEABLE","reviewDecision":"APPROVED","statusCheckRollup":[{"state":"SUCCESS"}]}`, nil
			case strings.Contains(s, branch2):
				return `{"number":42,"title":"Add API","state":"OPEN","mergeable":"MERGEABLE","reviewDecision":"APPROVED","statusCheckRollup":[{"state":"PENDING"}]}`, nil
			case strings.Contains(s, branch3):
				return `{"number":43,"title":"Add UI","state":"OPEN","isDraft":true,"mergeable":"CONFLICTING","reviewDecision":"","statusCheckRollup":[]}`, nil
			}
		}
		return "", nil
	}
	if code := app.Run(context.Background(), []string{"status", "--base", "main"}); code != 0 {
		t.Fatalf("code %d err=%s", code, errb.String())
	}
	want := `mystack  (base: main)
  ➖➖❌❌ #43     C3 Add UI
  ⌛✅✅✅ #42     C2 Add API
  ✅✅✅✅ #41     C1 Add model
   │ │ │ └─ stack: all PRs below are ready
   │ │ └─── conflicts: no merge conflicts
   │ └───── approval: PR is approved
   └─────── checks: CI checks pass
`
	if out.String() != want {
		t.Errorf("status stdout:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
}

func TestStatusJSON(t *testing.T) {
	app, f, out, _ := newTestApp()
	linked, creates := false, 0
	f.Handler = fullHandler(&linked, &creates, true)
	if code := app.Run(context.Background(), []string{"status", "--base", "main", "--json"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(out.String(), `"bookmark": "mystack"`) {
		t.Fatalf("out %q", out.String())
	}
}

func TestSyncWiring(t *testing.T) {
	app, f, out, _ := newTestApp()
	linked, creates := false, 0
	f.Handler = fullHandler(&linked, &creates, false)
	if code := app.Run(context.Background(), []string{"sync", "--base", "main"}); code != 0 {
		t.Fatalf("code %d out=%s", code, out.String())
	}
	if !callsContain(f, "rebase -s C1 -o main") {
		t.Fatalf("calls %v", f.Calls)
	}
}

func callsContain(f *runner.Fake, sub string) bool {
	for _, c := range f.Calls {
		if strings.Contains(c.String(), sub) {
			return true
		}
	}
	return false
}
