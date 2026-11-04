package stack

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
	"github.com/levibostian/jj-spr/internal/runner"
)

func commandContains(c runner.Command, parts ...string) bool {
	s := c.String()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func clients(handler func(runner.Command) (string, error)) (*jj.Client, *gh.Client, *runner.Fake) {
	f := &runner.Fake{Handler: handler}
	return &jj.Client{R: f}, &gh.Client{R: f, Repo: gh.Repo{Owner: "me", Name: "repo"}}, f
}

func commitLine(changeID, description string, empty bool) string {
	flag := "0"
	if empty {
		flag = "1"
	}
	return fmt.Sprintf(`{"commit_id":"sha-%s","change_id":%q,"description":%q}`, changeID, changeID, description) +
		"\x1f" + strings.Join([]string{flag, changeID, changeID}, "\x1f") + "\n"
}

func resolveHandler(bookmarks, logOut string, prs map[string]string) func(runner.Command) (string, error) {
	return func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "log", "remote_bookmarks"):
			return "main\n", nil
		case commandContains(c, "bookmark", "list"):
			return bookmarks, nil
		case commandContains(c, "log") && strings.Contains(c.String(), "~::trunk()"):
			return "C2\x1fmystack\n", nil
		case commandContains(c, "log"):
			return logOut, nil
		case commandContains(c, "pulls"):
			branch := ""
			for _, a := range c.Args {
				if strings.HasPrefix(a, "head=me:") {
					branch = strings.TrimPrefix(a, "head=me:")
				}
			}
			return prs[branch], nil
		}
		return "", fmt.Errorf("unexpected command: %s", c.String())
	}
}

func TestResolve(t *testing.T) {
	bookmarks := "mystack\tC2\nmystack-C1\tC1\nmystack-C2\tC2\nmain\tC0\n"
	logOut := commitLine("C1", "Add model\n", false) + commitLine("C2", "Add API\n\nBody\n", false)
	prs := map[string]string{
		"mystack-C1": `[{"number":41,"state":"open","title":"Add model","body":"","html_url":"u","head":{"ref":"mystack-C1"},"base":{"ref":"main"}}]`,
		"mystack-C2": "[]",
	}
	jjc, ghc, _ := clients(resolveHandler(bookmarks, logOut, prs))
	st, err := Resolve(context.Background(), jjc, ghc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Bookmark != "mystack" || st.HeadChangeID != "C2" || st.BaseBranch != "main" {
		t.Fatalf("got %+v", st)
	}
	if len(st.Entries) != 2 || st.Entries[0].PR == nil || st.Entries[0].PR.Number != 41 || st.Entries[1].PR != nil {
		t.Fatalf("got %+v", st.Entries)
	}
	if got := st.Active(); len(got) != 2 {
		t.Fatalf("active %d", len(got))
	}
}

func TestResolveExplicitStack(t *testing.T) {
	bookmarks := "mystack\tC2\nother\tX\n"
	jjc, ghc, _ := clients(resolveHandler(bookmarks, commitLine("C2", "x\n", false), map[string]string{"mystack-C2": "[]"}))
	if _, err := Resolve(context.Background(), jjc, ghc, Options{Stack: "missing"}); err == nil {
		t.Fatal("want error for unknown bookmark")
	}
}

func TestResolveNoBookmark(t *testing.T) {
	jjc, ghc, _ := clients(func(c runner.Command) (string, error) {
		switch {
		case commandContains(c, "log", "remote_bookmarks"):
			return "main\n", nil
		case commandContains(c, "bookmark", "list"):
			return "main\tC0\n", nil
		case commandContains(c, "log"):
			return "", nil
		}
		return "", fmt.Errorf("unexpected: %s", c.String())
	})
	if _, err := Resolve(context.Background(), jjc, ghc, Options{}); err == nil {
		t.Fatal("want error")
	}
}

// bookmarkHandler serves the three stack-bookmark revsets. ancestorRev,
// descendantRev and headRev are the raw "change-id\x1fnames" lines for the
// nearest-ancestor, nearest-descendant and stack-head queries; "" models "none
// found". The head query contains both markers, so it is matched first.
func bookmarkHandler(ancestorRev, descendantRev, headRev string) func(runner.Command) (string, error) {
	return func(c runner.Command) (string, error) {
		s := c.String()
		switch {
		case commandContains(c, "log", "remote_bookmarks"):
			return "main\n", nil
		case commandContains(c, "log") && strings.Contains(s, "::@ | @::"):
			return headRev, nil
		case commandContains(c, "log") && strings.Contains(s, "@::"):
			return descendantRev, nil
		case commandContains(c, "log") && strings.Contains(s, "~::"):
			return ancestorRev, nil
		case commandContains(c, "log"):
			return commitLine("C", "x\n", false), nil
		case commandContains(c, "pulls"):
			return "[]", nil
		}
		return "", fmt.Errorf("unexpected: %s", s)
	}
}

func TestResolveNearestAncestorBookmark(t *testing.T) {
	// @ is parked mid-stack next to a tool bookmark; the user bookmark above
	// names the stack and sets the top, so the whole stack resolves.
	jjc, ghc, _ := clients(bookmarkHandler("B\x1fmystack-B", "ignored\x1fother", "C\x1fmystack"))
	st, err := Resolve(context.Background(), jjc, ghc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Bookmark != "mystack" || st.HeadChangeID != "C" {
		t.Fatalf("got %q head %q", st.Bookmark, st.HeadChangeID)
	}
}

func TestResolveFallsBackToDescendantBookmark(t *testing.T) {
	jjc, ghc, _ := clients(bookmarkHandler("", "C\x1fother", "C\x1fother"))
	st, err := Resolve(context.Background(), jjc, ghc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Bookmark != "other" {
		t.Fatalf("got %q", st.Bookmark)
	}
}

func TestResolveMultipleBookmarks(t *testing.T) {
	jjc, ghc, _ := clients(bookmarkHandler("A\x1falpha beta", "", ""))
	_, err := Resolve(context.Background(), jjc, ghc, Options{})
	if err == nil || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("want ambiguity naming both, got %v", err)
	}
}

func TestActiveRetiresMerged(t *testing.T) {
	merged := "2024-01-01"
	e := Entry{Commit: jj.Commit{ChangeID: "x"}, PR: &gh.PR{Number: 1, State: "closed", MergedAt: &merged}}
	if e.Active() || !e.Retired() {
		t.Fatal("merged PR must be retired")
	}
	if (Entry{Commit: jj.Commit{Empty: true}}).Active() {
		t.Fatal("empty commit must not be active")
	}
}
