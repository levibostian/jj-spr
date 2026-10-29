package stack

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
)

func TestIsOrderedPrefix(t *testing.T) {
	cases := []struct {
		prefix, full []int
		want         bool
	}{
		{nil, []int{1}, true},
		{[]int{1}, []int{1, 2}, true},
		{[]int{1, 2}, []int{1, 2}, true},
		{[]int{2, 1}, []int{1, 2}, false},
		{[]int{1, 3}, []int{1, 2}, false},
		{[]int{1, 2, 3}, []int{1, 2}, false},
	}
	for _, c := range cases {
		if got := isOrderedPrefix(c.prefix, c.full); got != c.want {
			t.Fatalf("isOrderedPrefix(%v,%v)=%v want %v", c.prefix, c.full, got, c.want)
		}
	}
}

func TestReconcileEmpty(t *testing.T) {
	_, ghc, f := clients(func(runner.Command) (string, error) { return "", nil })
	if _, err := Reconcile(context.Background(), ghc, "main", nil); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 0 {
		t.Fatalf("want no calls, got %v", f.Calls)
	}
}

func TestReconcileSingleExistingStack(t *testing.T) {
	_, ghc, f := clients(func(c runner.Command) (string, error) {
		if commandContains(c, "stacks?pull_request") {
			return `[{"number":5,"pull_requests":[{"number":1,"state":"open"}]}]`, nil
		}
		return "", nil
	})
	if _, err := Reconcile(context.Background(), ghc, "main", []int{1}); err != nil {
		t.Fatal(err)
	}
	if !callsContain(f, "stacks/5/unstack") || !callsContain(f, "pr edit 1 --base main") {
		t.Fatalf("got %v", f.Calls)
	}
}

func TestReconcileSingleNoStack(t *testing.T) {
	_, ghc, f := clients(func(c runner.Command) (string, error) {
		if commandContains(c, "stacks?pull_request") {
			return "[]", nil
		}
		return "", nil
	})
	if _, err := Reconcile(context.Background(), ghc, "main", []int{1}); err != nil {
		t.Fatal(err)
	}
	if callsContain(f, "unstack") {
		t.Fatalf("should not unstack: %v", f.Calls)
	}
	if !callsContain(f, "pr edit 1 --base main") {
		t.Fatalf("got %v", f.Calls)
	}
}

func TestReconcileCreatesStack(t *testing.T) {
	linked := false
	_, ghc, f := clients(func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "stack link"):
			linked = true
			return "", nil
		case commandContains(c, "stacks?pull_request"):
			if linked {
				return `[{"number":9,"pull_requests":[{"number":1,"state":"open"},{"number":2,"state":"open"}]}]`, nil
			}
			return "[]", nil
		}
		return "", nil
	})
	n, err := Reconcile(context.Background(), ghc, "main", []int{1, 2})
	if err != nil || n != 9 {
		t.Fatalf("got %d %v", n, err)
	}
	if !callsContain(f, "stack link --base main 1 2") {
		t.Fatalf("got %v", f.Calls)
	}
}

func TestReconcileAppendsWithoutUnstack(t *testing.T) {
	_, ghc, f := clients(func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "stacks?pull_request"):
			return `[{"number":3,"pull_requests":[{"number":1,"state":"open"},{"number":2,"state":"open"}]}]`, nil
		}
		return "", nil
	})
	if _, err := Reconcile(context.Background(), ghc, "main", []int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if callsContain(f, "unstack") {
		t.Fatalf("should not unstack: %v", f.Calls)
	}
}

func TestReconcileRecreatesOnReorder(t *testing.T) {
	_, ghc, f := clients(func(c runner.Command) (string, error) {
		if commandContains(c, "stacks?pull_request") {
			return `[{"number":3,"pull_requests":[{"number":2,"state":"open"},{"number":1,"state":"open"}]}]`, nil
		}
		return "", nil
	})
	if _, err := Reconcile(context.Background(), ghc, "main", []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	if !callsContain(f, "stacks/3/unstack") {
		t.Fatalf("want unstack: %v", f.Calls)
	}
}

func TestReconcileRecreatesWhenStackHasMerged(t *testing.T) {
	_, ghc, f := clients(func(c runner.Command) (string, error) {
		if commandContains(c, "stacks?pull_request") {
			return `[{"number":3,"pull_requests":[{"number":1,"state":"closed","merged_at":"2024-01-01"},{"number":2,"state":"open"}]}]`, nil
		}
		return "", nil
	})
	if _, err := Reconcile(context.Background(), ghc, "main", []int{2, 3}); err != nil {
		t.Fatal(err)
	}
	if !callsContain(f, "stacks/3/unstack") {
		t.Fatalf("want unstack: %v", f.Calls)
	}
}

func TestPublishUpdatesExistingPRs(t *testing.T) {
	bookmarks := "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\n"
	logOut := commitLine("C1", "New title\n", false) + commitLine("C2", "Second\n", false)
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C2\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C1"):
			return `[{"number":41,"state":"open","title":"Old","body":"","html_url":"u","head":{"ref":"mystack-C1"},"base":{"ref":"main"}}]`, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C2"):
			return `[{"number":42,"state":"open","title":"Old2","body":"","html_url":"u","head":{"ref":"mystack-C2"},"base":{"ref":"mystack-C1"}}]`, nil
		case commandContains(c, "stacks?pull_request"):
			return `[{"number":3,"pull_requests":[{"number":41,"state":"open"},{"number":42,"state":"open"}]}]`, nil
		}
		return "", nil
	}
	jjc, ghc, f := clients(handler)
	res, err := Update(context.Background(), jjc, ghc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 2 || len(res.Created) != 0 {
		t.Fatalf("got %+v", res)
	}
	if !callsContain(f, "pr edit 41 --title New title") {
		t.Fatalf("missing title edit: %v", f.Calls)
	}
	if !callsContain(f, "pr edit 42 --title Second") {
		t.Fatalf("missing title edit: %v", f.Calls)
	}
}

// TestPublishRetargetsBaseBeforePushOnReorder is the regression test for
// GitHub auto-merging a PR whose force-pushed head becomes an ancestor of its
// base. On a reorder the down-mover's base must be changed before the push.
func TestPublishRetargetsBaseBeforePushOnReorder(t *testing.T) {
	bookmarks := "mystack\tC3\nmystack-C1\tC1\nmystack-C2\tC2\nmystack-C3\tC3\n"
	// New local order is C1, C3, C2; the GitHub stack is still [41, 42, 43].
	logOut := commitLine("C1", "One\n", false) + commitLine("C3", "Three\n", false) + commitLine("C2", "Two\n", false)
	unstacked, linked := false, false
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C3\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C1"):
			return `[{"number":41,"state":"open","title":"One","body":"","html_url":"u","head":{"ref":"mystack-C1"},"base":{"ref":"main"}}]`, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C2"):
			return `[{"number":42,"state":"open","title":"Two","body":"","html_url":"u","head":{"ref":"mystack-C2"},"base":{"ref":"mystack-C1"}}]`, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C3"):
			return `[{"number":43,"state":"open","title":"Three","body":"","html_url":"u","head":{"ref":"mystack-C3"},"base":{"ref":"mystack-C2"}}]`, nil
		case commandContains(c, "stacks?pull_request"):
			if linked {
				return `[{"number":6,"pull_requests":[{"number":41,"state":"open"},{"number":43,"state":"open"},{"number":42,"state":"open"}]}]`, nil
			}
			if unstacked {
				return "[]", nil
			}
			return `[{"number":5,"pull_requests":[{"number":41,"state":"open"},{"number":42,"state":"open"},{"number":43,"state":"open"}]}]`, nil
		case commandContains(c, "stacks/5/unstack"):
			unstacked = true
			return "", nil
		case commandContains(c, "stack link"):
			linked = true
			return "", nil
		}
		return "", nil
	}
	jjc, ghc, f := clients(handler)
	if _, err := Update(context.Background(), jjc, ghc, Options{}); err != nil {
		t.Fatal(err)
	}
	unstackAt, editAt, pushAt := callIndex(f, "stacks/5/unstack"), callIndex(f, "pr edit 43 --base mystack-C1"), callIndex(f, "git push")
	if unstackAt < 0 || editAt < 0 || pushAt < 0 {
		t.Fatalf("missing calls: unstack=%d edit=%d push=%d\n%v", unstackAt, editAt, pushAt, f.Calls)
	}
	if !(unstackAt < editAt && editAt < pushAt) {
		t.Fatalf("want unstack < retarget < push, got %d %d %d\n%v", unstackAt, editAt, pushAt, f.Calls)
	}
	if callsContain(f, "pr edit 42 --base") {
		t.Fatalf("up-mover must not be retargeted before push: %v", f.Calls)
	}
}

// TestPublishRetargetsBaseOnMidStackRemoval covers the other case that
// produces down-movers: a commit is removed from the middle of the stack, so
// every PR above it moves down and must be retargeted before the push.
func TestPublishRetargetsBaseOnMidStackRemoval(t *testing.T) {
	bookmarks := "mystack\tC3\nmystack-C1\tC1\nmystack-C3\tC3\n"
	// New local order is C1, C3; the GitHub stack is still [41, 42, 43].
	logOut := commitLine("C1", "One\n", false) + commitLine("C3", "Three\n", false)
	unstacked := false
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C3\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C1"):
			return `[{"number":41,"state":"open","title":"One","body":"","html_url":"u","head":{"ref":"mystack-C1"},"base":{"ref":"main"}}]`, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C3"):
			return `[{"number":43,"state":"open","title":"Three","body":"","html_url":"u","head":{"ref":"mystack-C3"},"base":{"ref":"mystack-C2"}}]`, nil
		case commandContains(c, "stacks?pull_request"):
			if unstacked {
				return "[]", nil
			}
			return `[{"number":5,"pull_requests":[{"number":41,"state":"open"},{"number":42,"state":"open"},{"number":43,"state":"open"}]}]`, nil
		case commandContains(c, "stacks/5/unstack"):
			unstacked = true
			return "", nil
		}
		return "", nil
	}
	jjc, ghc, f := clients(handler)
	if _, err := Update(context.Background(), jjc, ghc, Options{}); err != nil {
		t.Fatal(err)
	}
	unstackAt, editAt, pushAt := callIndex(f, "stacks/5/unstack"), callIndex(f, "pr edit 43 --base mystack-C1"), callIndex(f, "git push")
	if !(unstackAt >= 0 && unstackAt < editAt && editAt < pushAt) {
		t.Fatalf("want unstack < retarget < push, got %d %d %d\n%v", unstackAt, editAt, pushAt, f.Calls)
	}
	if !callsContain(f, "stack link --base main 41 43") {
		t.Fatalf("want stack relinked without the removed PR: %v", f.Calls)
	}
}

// TestPublishAppendDoesNotResetOrRetarget guards against over-reacting: a pure
// append must not unstack or retarget anything.
func TestPublishAppendDoesNotResetOrRetarget(t *testing.T) {
	bookmarks := "mystack\tC3\nmystack-C1\tC1\nmystack-C2\tC2\nmystack-C3\tC3\n"
	logOut := commitLine("C1", "One\n", false) + commitLine("C2", "Two\n", false) + commitLine("C3", "Three\n", false)
	linked := false
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C3\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C1"):
			return `[{"number":41,"state":"open","title":"One","body":"","html_url":"u","head":{"ref":"mystack-C1"},"base":{"ref":"main"}}]`, nil
		case commandContains(c, "pulls") && strings.Contains(c.String(), "mystack-C2"):
			return `[{"number":42,"state":"open","title":"Two","body":"","html_url":"u","head":{"ref":"mystack-C2"},"base":{"ref":"mystack-C1"}}]`, nil
		case commandContains(c, "pulls"):
			return "[]", nil
		case commandContains(c, "pr create"):
			return "https://github.com/me/repo/pull/43\n", nil
		case commandContains(c, "stacks?pull_request"):
			if linked {
				return `[{"number":5,"pull_requests":[{"number":41,"state":"open"},{"number":42,"state":"open"},{"number":43,"state":"open"}]}]`, nil
			}
			return `[{"number":5,"pull_requests":[{"number":41,"state":"open"},{"number":42,"state":"open"}]}]`, nil
		case commandContains(c, "stack link"):
			linked = true
			return "", nil
		}
		return "", nil
	}
	jjc, ghc, f := clients(handler)
	if _, err := Update(context.Background(), jjc, ghc, Options{}); err != nil {
		t.Fatal(err)
	}
	if callsContain(f, "unstack") {
		t.Fatalf("append must not unstack: %v", f.Calls)
	}
	if callsContain(f, "pr edit") {
		t.Fatalf("append must not retarget: %v", f.Calls)
	}
	if !callsContain(f, "stack link --base main 41 42 43") {
		t.Fatalf("append must link the new PR: %v", f.Calls)
	}
}

func callIndex(f *runner.Fake, sub string) int {
	for i, c := range f.Calls {
		if strings.Contains(c.String(), sub) {
			return i
		}
	}
	return -1
}

func callsContain(f *runner.Fake, sub string) bool {
	for _, c := range f.Calls {
		if strings.Contains(c.String(), sub) {
			return true
		}
	}
	return false
}

func TestPublishCreatesPRsAndStack(t *testing.T) {
	bookmarks := "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\n"
	logOut := commitLine("C1", "Add model\n", false) + commitLine("C2", "Add API\n", false)
	linked := false
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C2\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls"):
			return "[]", nil
		case commandContains(c, "stack link"):
			linked = true
			return "", nil
		case commandContains(c, "stacks?pull_request"):
			if linked {
				return `[{"number":9,"pull_requests":[{"number":1,"state":"open"},{"number":2,"state":"open"}]}]`, nil
			}
			return "[]", nil
		}
		return "", nil
	}
	jjc, ghc, _ := clients(handler)
	// Give each create a distinct URL by counting creates.
	createCount := 0
	f := jjc.R.(*runner.Fake)
	base := f.Handler
	f.Handler = func(c runner.Command) (string, error) {
		if commandContains(c, "pr create") {
			createCount++
			return fmt.Sprintf("https://github.com/me/repo/pull/%d\n", createCount), nil
		}
		return base(c)
	}

	res, err := Update(context.Background(), jjc, ghc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 2 || res.StackNumber != 9 {
		t.Fatalf("got %+v", res)
	}
	if !callsContain(f, "bookmark set --allow-backwards -r C1 mystack-C1") {
		t.Fatalf("missing bookmark set: %v", f.Calls)
	}
	if !callsContain(f, "git push --remote origin --allow-empty-description -b mystack-C1 -b mystack-C2") {
		t.Fatalf("missing push: %v", f.Calls)
	}
}
