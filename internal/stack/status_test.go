package stack

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
	"github.com/levibostian/jj-spr/internal/runner"
)

func TestChecksBit(t *testing.T) {
	cases := []struct {
		checks []gh.Check
		want   string
	}{
		{nil, "none"},
		{[]gh.Check{{State: "SUCCESS"}}, "pass"},
		{[]gh.Check{{Conclusion: "FAILURE"}}, "fail"},
		{[]gh.Check{{Status: "IN_PROGRESS"}}, "pending"},
		{[]gh.Check{{Conclusion: "SUCCESS"}, {State: "PENDING"}}, "pending"},
		{[]gh.Check{{Conclusion: "SUCCESS"}, {Conclusion: "FAILURE"}}, "fail"},
	}
	for _, c := range cases {
		if got := checksBit(&gh.PR{Checks: c.checks}); got != c.want {
			t.Fatalf("checksBit(%+v)=%s want %s", c.checks, got, c.want)
		}
	}
}

func TestReviewAndMergeBits(t *testing.T) {
	if reviewBit(&gh.PR{ReviewDecision: "APPROVED"}) != "approved" ||
		reviewBit(&gh.PR{ReviewDecision: "CHANGES_REQUESTED"}) != "changes" ||
		reviewBit(&gh.PR{}) != "none" {
		t.Fatal("review bits wrong")
	}
	if mergeBit(&gh.PR{MergeStateStatus: "CLEAN"}) != "clean" ||
		mergeBit(&gh.PR{MergeStateStatus: "DIRTY"}) != "conflicts" ||
		mergeBit(&gh.PR{MergeStateStatus: "DRAFT"}) != "draft" ||
		mergeBit(&gh.PR{}) != "unknown" {
		t.Fatal("merge bits wrong")
	}
}

func TestBitsForNil(t *testing.T) {
	if bitsFor(nil).String() != "➖➖➖➖" {
		t.Fatalf("got %s", bitsFor(nil).String())
	}
}

func TestShortID(t *testing.T) {
	c := jj.Commit{ChangeID: "zwwzlzkyuvsr", IDPrefix: "z", ShortID: "zwwzlzky"}
	if got := shortID(c, false); got != "zwwzlzky" {
		t.Fatalf("plain %q", got)
	}
	want := "\x1b[1m\x1b[38;5;5mz\x1b[0m\x1b[38;5;8mwwzlzky\x1b[39m"
	if got := shortID(c, true); got != want {
		t.Fatalf("color %q want %q", got, want)
	}
	// No ShortID falls back to the full change ID.
	if got := shortID(jj.Commit{ChangeID: "abc"}, true); got != "abc" {
		t.Fatalf("fallback %q", got)
	}
}

func TestBuildReport(t *testing.T) {
	view := `{"number":41,"title":"Add model","state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","reviewDecision":"APPROVED","statusCheckRollup":[{"state":"SUCCESS"}],"url":"u","baseRefName":"main"}`
	_, ghc, _ := clients(func(c runner.Command) (string, error) {
		if commandContains(c, "pr view") {
			return view, nil
		}
		return "", fmt.Errorf("unexpected: %s", c.String())
	})
	merged := "2024-01-01"
	st := &Stack{Bookmark: "mystack", BaseBranch: "main", Entries: []Entry{
		{Commit: jj.Commit{ChangeID: "C1", Description: "Add model\n", ShortID: "C1x", IDPrefix: "C"}, Branch: "mystack-C1", PR: &gh.PR{Number: 40, State: "closed", MergedAt: &merged}},
		{Commit: jj.Commit{ChangeID: "C2", Description: "Add API\n\nbody\n", ShortID: "C2x", IDPrefix: "C"}, Branch: "mystack-C2", PR: &gh.PR{Number: 41, State: "open"}},
	}}
	r, err := BuildReport(context.Background(), ghc, st)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows[0].Bits.Stack != "clear" || r.Rows[1].Bits.Checks != "pass" {
		t.Fatalf("got %+v", r.Rows)
	}
	var buf bytes.Buffer
	Render(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "#41") || !strings.Contains(out, "mystack") {
		t.Fatalf("render: %s", out)
	}
	if !strings.Contains(out, "C2x Add API") {
		t.Fatalf("want short id + title: %s", out)
	}
	if strings.Contains(out, "mystack-C1") {
		t.Fatalf("branch should not render: %s", out)
	}
	for _, label := range []string{"checks:", "approval:", "conflicts:", "stack:"} {
		if !strings.Contains(out, label) {
			t.Fatalf("legend missing %q: %s", label, out)
		}
	}
	b, err := r.JSON()
	if err != nil || !strings.Contains(string(b), "mystack") {
		t.Fatalf("json %s %v", b, err)
	}
}

func TestAllGreen(t *testing.T) {
	if !allGreen(nil) {
		t.Fatal("empty is green")
	}
	rows := []StatusRow{{Entry: Entry{Commit: jj.Commit{ChangeID: "x"}}, Bits: Bits{Checks: "fail", Merge: "clean"}}}
	if allGreen(rows) {
		t.Fatal("failed checks must not be green")
	}
	rows[0].Bits = Bits{Checks: "pass", Merge: "conflicts"}
	if allGreen(rows) {
		t.Fatal("conflicts must not be green")
	}
}
