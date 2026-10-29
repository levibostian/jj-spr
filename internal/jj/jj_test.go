package jj

import (
	"context"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
)

func run(t *testing.T, out string, err error) (*Client, *runner.Fake) {
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
	out := `{"commit_id":"a","change_id":"x","description":"one\n"}` + "\x1f" + "0\n" +
		`{"commit_id":"b","change_id":"y","description":"two\n\nbody\n"}` + "\x1f" + "1\n"
	c, _ := run(t, out, nil)
	got, err := c.Log(context.Background(), "trunk()..H")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ChangeID != "x" || got[1].Empty != true {
		t.Fatalf("got %+v", got)
	}
}

func TestLogRejectsMalformed(t *testing.T) {
	c, _ := run(t, "not json\x1f0\n", nil)
	if _, err := c.Log(context.Background(), "r"); err == nil {
		t.Fatal("want error")
	}
}

func TestBookmarksParse(t *testing.T) {
	c, _ := run(t, "mystack\tx\nremote@origin\t\nmystack/y\ty\n", nil)
	got, err := c.Bookmarks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "mystack" || got[1].ChangeID != "y" {
		t.Fatalf("got %+v", got)
	}
}

func TestSetBookmarkArgs(t *testing.T) {
	c, f := run(t, "", nil)
	if err := c.SetBookmark(context.Background(), "mystack/x", "x"); err != nil {
		t.Fatal(err)
	}
	want := "jj bookmark set --allow-backwards -r x mystack/x"
	if f.Calls[0].String() != want {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestPush(t *testing.T) {
	c, f := run(t, "", nil)
	if err := c.Push(context.Background(), "origin", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Calls[0].String(), "git push --remote origin --allow-empty-description -b a -b b") {
		t.Fatalf("got %q", f.Calls[0].String())
	}
}

func TestPushNoBranches(t *testing.T) {
	c, f := run(t, "", nil)
	if err := c.Push(context.Background(), "origin", nil); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 0 {
		t.Fatal("expected no calls")
	}
}

func TestFetchRebase(t *testing.T) {
	c, f := run(t, "", nil)
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
	c, _ := run(t, "abc\n", nil)
	ok, err := c.Match(context.Background(), "r")
	if err != nil || !ok {
		t.Fatalf("got %v %v", ok, err)
	}
	c2, _ := run(t, "\n", nil)
	if ok, _ := c2.Match(context.Background(), "r"); ok {
		t.Fatal("want false")
	}
}

func TestConflicts(t *testing.T) {
	c, _ := run(t, "aaa\nbbb\n", nil)
	got, err := c.Conflicts(context.Background(), "trunk()..H")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v %v", got, err)
	}
}
