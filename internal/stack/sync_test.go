package stack

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
)

func TestSync(t *testing.T) {
	bookmarks := "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\n"
	logOut := commitLine("C1", "Add model\n", false) + commitLine("C2", "Add API\n", false)
	linked, creates := false, 0
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
		case commandContains(c, "log") && strings.Contains(c.String(), "conflicts()"):
			return "", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls"):
			return "[]", nil
		case commandContains(c, "pr create"):
			creates++
			return fmt.Sprintf("https://github.com/me/repo/pull/%d\n", creates), nil
		case commandContains(c, "stack link"):
			linked = true
			return "", nil
		case commandContains(c, "stacks?pull_request"):
			if linked {
				return `[{"number":4,"pull_requests":[{"number":1,"state":"open"},{"number":2,"state":"open"}]}]`, nil
			}
			return "[]", nil
		}
		return "", nil
	}
	jjc, ghc, f := clients(handler)
	res, err := Sync(context.Background(), jjc, ghc, Options{Remote: "origin"})
	if err != nil {
		t.Fatal(err)
	}
	if !callsContain(f, "git fetch --remote origin") || !callsContain(f, "rebase -s C1 -o trunk()") {
		t.Fatalf("missing fetch/rebase: %v", f.Calls)
	}
	if len(res.Created) != 2 || res.StackNumber != 4 {
		t.Fatalf("got %+v", res)
	}
}

func TestStatusHelper(t *testing.T) {
	bookmarks := "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\n"
	logOut := commitLine("C1", "Add model\n", false)
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C2\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "pulls"):
			return "[]", nil
		}
		return "", nil
	}
	jjc, ghc, _ := clients(handler)
	r, err := Status(context.Background(), jjc, ghc, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Stack.Bookmark != "mystack" || len(r.Rows) != 1 {
		t.Fatalf("got %+v", r)
	}
}

func TestSyncStopsOnConflicts(t *testing.T) {
	bookmarks := "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\n"
	logOut := commitLine("C1", "Add model\n", false)
	handler := func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "repo view", "defaultBranchRef"):
			return "main\n", nil
		case commandContains(c, "repo view", "owner"):
			return "me\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "conflicts()"):
			return "C1\n", nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C2\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls"):
			return "[]", nil
		}
		return "", nil
	}
	jjc, ghc, _ := clients(handler)
	if _, err := Sync(context.Background(), jjc, ghc, Options{}); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("want conflict error, got %v", err)
	}
}
