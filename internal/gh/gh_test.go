package gh

import (
	"context"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
)

func fake(handler func(runner.Command) (string, error)) (*Client, *runner.Fake) {
	f := &runner.Fake{Handler: handler}
	return &Client{R: f}, f
}

func TestAvailable(t *testing.T) {
	c, _ := fake(func(runner.Command) (string, error) { return "", nil })
	if err := c.Available(context.Background()); err != nil {
		t.Fatal(err)
	}
	c2, _ := fake(func(runner.Command) (string, error) { return "", context.DeadlineExceeded })
	if err := c2.Available(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

func TestOwnerAndDefaultBranch(t *testing.T) {
	c, f := fake(func(runner.Command) (string, error) { return "levibostian\n", nil })
	if got, _ := c.Owner(context.Background()); got != "levibostian" {
		t.Fatalf("got %q", got)
	}
	if got, _ := c.DefaultBranch(context.Background()); got != "levibostian" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(f.Calls[0].String(), "repo view") {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestFindPR(t *testing.T) {
	out := `[{"number":42,"state":"open","draft":false,"title":"t","body":"b","html_url":"https://x/pull/42","head":{"ref":"mystack/x"},"base":{"ref":"main"}}]`
	c, f := fake(func(runner.Command) (string, error) { return out, nil })
	pr, err := c.FindPR(context.Background(), "levibostian", "mystack/x")
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || pr.Number != 42 || pr.HeadRefName != "mystack/x" {
		t.Fatalf("got %+v", pr)
	}
	call := f.Calls[0].String()
	if !strings.Contains(call, "head=levibostian:mystack/x") || !strings.Contains(call, "--method GET") {
		t.Fatalf("got %q", call)
	}
}

func TestFindPRNone(t *testing.T) {
	c, _ := fake(func(runner.Command) (string, error) { return "[]", nil })
	pr, err := c.FindPR(context.Background(), "o", "b")
	if err != nil || pr != nil {
		t.Fatalf("got %+v %v", pr, err)
	}
}

func TestCreatePR(t *testing.T) {
	c, f := fake(func(runner.Command) (string, error) { return "https://github.com/o/r/pull/7\n", nil })
	n, err := c.CreatePR(context.Background(), "mystack/x", "main", "title", "body", true)
	if err != nil || n != 7 {
		t.Fatalf("got %d %v", n, err)
	}
	if !strings.Contains(f.Calls[0].String(), "--draft") || f.Calls[0].Stdin != "body" {
		t.Fatalf("got %+v", f.Calls[0])
	}
}

func TestCreatePRBadOutput(t *testing.T) {
	c, _ := fake(func(runner.Command) (string, error) { return "nope", nil })
	if _, err := c.CreatePR(context.Background(), "h", "b", "t", "", false); err == nil {
		t.Fatal("want error")
	}
}

func TestEditPR(t *testing.T) {
	c, f := fake(func(runner.Command) (string, error) { return "", nil })
	if err := c.EditPR(context.Background(), 3, "t", "b"); err != nil {
		t.Fatal(err)
	}
	if f.Calls[0].Stdin != "b" || !strings.Contains(f.Calls[0].String(), "pr edit 3") {
		t.Fatalf("got %+v", f.Calls[0])
	}
	if err := c.EditPRBase(context.Background(), 3, "main"); err != nil {
		t.Fatal(err)
	}
}

func TestViewPR(t *testing.T) {
	out := `{"number":1,"title":"t","state":"OPEN","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","reviewDecision":"APPROVED","statusCheckRollup":[{"state":"SUCCESS"}],"url":"u","baseRefName":"main"}`
	c, _ := fake(func(runner.Command) (string, error) { return out, nil })
	pr, err := c.ViewPR(context.Background(), "mystack/x")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 1 || pr.Checks[0].State != "SUCCESS" || pr.ReviewDecision != "APPROVED" {
		t.Fatalf("got %+v", pr)
	}
}

func TestRemoteStackHelpers(t *testing.T) {
	out := `[{"number":9,"pull_requests":[{"number":1,"state":"closed","merged_at":"2024-01-01"},{"number":2,"state":"open","merged_at":null}]}]`
	c, _ := fake(func(runner.Command) (string, error) { return out, nil })
	s, err := c.FindStackForPR(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if s.Number != 9 || !s.HasMerged() || len(s.OpenPRs()) != 1 || s.OpenPRs()[0] != 2 {
		t.Fatalf("got %+v open=%v", s, s.OpenPRs())
	}
	c2, _ := fake(func(runner.Command) (string, error) { return "[]", nil })
	if s, _ := c2.FindStackForPR(context.Background(), 2); s != nil {
		t.Fatal("want nil")
	}
}

func TestUnstack(t *testing.T) {
	c, f := fake(func(runner.Command) (string, error) { return "", nil })
	if err := c.Unstack(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Calls[0].String(), "stacks/9/unstack") {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestStackLink(t *testing.T) {
	c, f := fake(func(runner.Command) (string, error) { return "", nil })
	if err := c.StackLink(context.Background(), "main", []int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	got := f.Calls[0].String()
	if !strings.Contains(got, "stack link --base main 1 2 3") {
		t.Fatalf("got %q", got)
	}
}
