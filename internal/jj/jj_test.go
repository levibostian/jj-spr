package jj

import (
	"context"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
)

func newClient(t *testing.T, out string, err error) (*Client, *runner.Fake) {
	t.Helper()
	f := &runner.Fake{Handler: func(runner.Command) (string, error) { return out, err }}
	return &Client{R: f}, f
}

func TestCommitTitleBody(t *testing.T) {
	c := Commit{Description: "Add model\n\nDetails here\nmore"}
	if c.Title() != "Add model" {
		t.Fatalf("title %q", c.Title())
	}
	if c.Body() != "Details here\nmore" {
		t.Fatalf("body %q", c.Body())
	}
	if (Commit{Description: "only"}).Body() != "" {
		t.Fatal("want empty body")
	}
}

func TestLogParsesCommits(t *testing.T) {
	sep := "\x1f"
	out := strings.Join([]string{`{"commit_id":"a","change_id":"x","description":"one\n"}`, "0", "x", "xywxyzab"}, sep) + "\n" +
		strings.Join([]string{`{"commit_id":"b","change_id":"y","description":"two\n\nbody\n"}`, "1", "y", "ywwzlzky"}, sep) + "\n"
	c, _ := newClient(t, out, nil)
	got, err := c.Log(context.Background(), "trunk()..H")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ChangeID != "x" || got[1].Empty != true {
		t.Fatalf("got %+v", got)
	}
	if got[1].IDPrefix != "y" || got[1].ShortID != "ywwzlzky" {
		t.Fatalf("short id wrong: %+v", got[1])
	}
}

func TestLogRejectsMalformed(t *testing.T) {
	c, _ := newClient(t, "not json\x1f0\x1fx\x1fxyw\n", nil)
	if _, err := c.Log(context.Background(), "r"); err == nil {
		t.Fatal("want error")
	}
}

func TestBookmarksParse(t *testing.T) {
	c, _ := newClient(t, "mystack\tx\nremote@origin\t\nmystack/y\ty\n", nil)
	got, err := c.Bookmarks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "mystack" || got[1].ChangeID != "y" {
		t.Fatalf("got %+v", got)
	}
}

func TestRemoteURL(t *testing.T) {
	c, f := newClient(t, "origin git@github.com:levibostian/jj-spr.git\nupstream https://github.com/jj-vcs/jj.git\n", nil)
	got, err := c.RemoteURL(context.Background(), "upstream")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/jj-vcs/jj.git" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(f.Calls[0].String(), "git remote list") {
		t.Fatalf("got %q", f.Calls[0].String())
	}
	if _, err := c.RemoteURL(context.Background(), "missing"); err == nil {
		t.Fatal("want error for a missing remote")
	}
}

// stackHandler serves the three stack-bookmark revsets: the nearest-ancestor
// query, the nearest-descendant query, and the head query. The head query
// contains both "::@" and "@::", so it is matched first; the descendant query
// also contains "~::", so it is matched before the ancestor.
func stackHandler(ancestor, descendant, head string) *runner.Fake {
	return &runner.Fake{Handler: func(c runner.Command) (string, error) {
		s := c.String()
		switch {
		case strings.Contains(s, "::@ | @::"):
			return head, nil
		case strings.Contains(s, "@::"):
			return descendant, nil
		case strings.Contains(s, "~::"):
			return ancestor, nil
		}
		return "", nil
	}}
}

func TestStackBookmarkResolvesStackTop(t *testing.T) {
	// @ sits on B next to a tool bookmark; the user bookmark at C names the
	// stack top. The result must be C, so the whole stack is published.
	c := &Client{R: stackHandler("B\x1fmystack-B", "ignored\x1fother", "C\x1fmystack")}
	got, err := c.StackBookmark(context.Background(), "trunk()")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "mystack" || got.ChangeID != "C" {
		t.Fatalf("got %+v", got)
	}
}

func TestStackBookmarkHeadFromToolBookmark(t *testing.T) {
	// No user bookmark: the topmost tool bookmark names the stack top.
	c := &Client{R: stackHandler("B\x1fmystack-B", "", "C\x1fmystack-C")}
	got, err := c.StackBookmark(context.Background(), "trunk()")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "mystack" || got.ChangeID != "C" {
		t.Fatalf("got %+v", got)
	}
}

func TestStackBookmarkFallsBackToDescendant(t *testing.T) {
	c := &Client{R: stackHandler("", "C\x1fother", "C\x1fother")}
	got, err := c.StackBookmark(context.Background(), "trunk()")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "other" || got.ChangeID != "C" {
		t.Fatalf("got %+v", got)
	}
}

func TestStackBookmarkIgnoresRemote(t *testing.T) {
	c := &Client{R: stackHandler("A\x1fmystack me@origin", "", "A\x1fmystack")}
	got, err := c.StackBookmark(context.Background(), "trunk()")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "mystack" {
		t.Fatalf("got %+v", got)
	}
}

func TestStackBookmarkIgnoresDivergedMarker(t *testing.T) {
	// jj renders a local bookmark that diverged from its remote with a trailing
	// "*". The raw name must come from bookmarks.name(), not the rendered join.
	wantName := "mystack"
	f := &runner.Fake{Handler: func(c runner.Command) (string, error) {
		s := c.String()
		name := wantName
		if strings.Contains(s, "bookmarks.join") {
			name += "*" // emulate jj's diverged-bookmark marker
		}
		commitLine := "A\x1f" + name
		switch {
		case strings.Contains(s, "::@ | @::"):
			return commitLine, nil
		case strings.Contains(s, "@::"):
			return "", nil
		case strings.Contains(s, "~::"):
			return commitLine, nil
		}
		return "", nil
	}}
	c := &Client{R: f}
	got, err := c.StackBookmark(context.Background(), "trunk()")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != wantName {
		t.Fatalf("got %+v", got)
	}
}

func TestStackBookmarkAmbiguous(t *testing.T) {
	c := &Client{R: stackHandler("A\x1falpha beta", "", "")}
	_, err := c.StackBookmark(context.Background(), "trunk()")
	if err == nil || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("want ambiguity naming both, got %v", err)
	}
}

func TestStackBookmarkNone(t *testing.T) {
	c := &Client{R: stackHandler("", "", "")}
	if _, err := c.StackBookmark(context.Background(), "trunk()"); err == nil {
		t.Fatal("want error")
	}
}

func TestWorkspace(t *testing.T) {
	f := &runner.Fake{Handler: func(c runner.Command) (string, error) {
		if strings.Contains(c.String(), "workspace list") {
			return "default\tA\nother\tB\n", nil
		}
		return "B\n", nil
	}}
	got, err := (&Client{R: f}).Workspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "other" {
		t.Fatalf("got %q", got)
	}
}

func TestSetBookmarkArgs(t *testing.T) {
	c, f := newClient(t, "", nil)
	if err := c.SetBookmark(context.Background(), "mystack/x", "x"); err != nil {
		t.Fatal(err)
	}
	want := "jj bookmark set --allow-backwards -r x mystack/x"
	if f.Calls[0].String() != want {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestPush(t *testing.T) {
	c, f := newClient(t, "", nil)
	if err := c.Push(context.Background(), "origin", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Calls[0].String(), "git push --remote origin --allow-empty-description -b a -b b") {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestPushNoBranches(t *testing.T) {
	c, f := newClient(t, "", nil)
	if err := c.Push(context.Background(), "origin", nil); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 0 {
		t.Fatal("expected no calls")
	}
}

func TestFetchRebase(t *testing.T) {
	c, f := newClient(t, "", nil)
	if err := c.Fetch(context.Background(), "origin"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rebase(context.Background(), "src", "trunk()"); err != nil {
		t.Fatal(err)
	}
	if f.Calls[1].String() != "jj rebase -s src -o trunk()" {
		t.Fatalf("got %q", f.Calls[1].String())
	}
}

func TestMatch(t *testing.T) {
	c, _ := newClient(t, "abc\n", nil)
	ok, err := c.Match(context.Background(), "r")
	if err != nil || !ok {
		t.Fatalf("got %v %v", ok, err)
	}
	c2, _ := newClient(t, "\n", nil)
	if ok, _ := c2.Match(context.Background(), "r"); ok {
		t.Fatal("want false")
	}
}

func TestRoot(t *testing.T) {
	c, f := newClient(t, "/repo/root\n", nil)
	root, err := c.Root(context.Background())
	if err != nil || root != "/repo/root" {
		t.Fatalf("got %q %v", root, err)
	}
	if f.Calls[0].String() != "jj root" {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestConflicts(t *testing.T) {
	c, _ := newClient(t, "aaa\nbbb\n", nil)
	got, err := c.Conflicts(context.Background(), "trunk()..H")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v %v", got, err)
	}
}
