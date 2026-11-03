// Package stack resolves a jj bookmark into a GitHub stack and publishes it.
package stack

import (
	"context"
	"fmt"
	"strings"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
)

// Entry is one commit in the stack.
type Entry struct {
	Commit jj.Commit `json:"commit"`
	Branch string    `json:"branch"`
	PR     *gh.PR    `json:"pr,omitempty"`
}

// Retired reports whether the entry's PR is merged or closed.
func (e Entry) Retired() bool { return e.PR != nil && e.PR.Retired() }

// Active reports whether the entry should be published.
func (e Entry) Active() bool { return !e.Commit.Empty && !e.Retired() }

// Stack is the resolved view of one user bookmark.
type Stack struct {
	Bookmark     string  `json:"bookmark"`
	HeadChangeID string  `json:"headChangeId"`
	BaseBranch   string  `json:"baseBranch"`
	Entries      []Entry `json:"entries"`
}

// Active returns the publishable entries, bottom to top.
func (s *Stack) Active() []Entry {
	var out []Entry
	for _, e := range s.Entries {
		if e.Active() {
			out = append(out, e)
		}
	}
	return out
}

// Options are the user-supplied knobs shared by all commands.
type Options struct {
	Stack  string
	Base   string
	Remote string
	Draft  bool
}

func (o Options) remote() string {
	if o.Remote == "" {
		return "origin"
	}
	return o.Remote
}

// baseRev is the revset of the stack base. --base names a branch or bookmark;
// the default is jj's trunk(). Everything the stack is not sits below it.
func baseRev(opts Options) string {
	if opts.Base != "" {
		return opts.Base
	}
	return "trunk()"
}

// Resolve builds the stack for a bookmark. When Stack is empty the single
// non-tool bookmark strictly after trunk is chosen.
func Resolve(ctx context.Context, jjc *jj.Client, ghc *gh.Client, opts Options) (*Stack, error) {
	base := opts.Base
	if base == "" {
		var err error
		if base, err = jjc.TrunkBranch(ctx, opts.remote()); err != nil {
			return nil, err
		}
	}
	bookmarks, err := jjc.Bookmarks(ctx)
	if err != nil {
		return nil, err
	}
	bookmark, err := chooseBookmark(ctx, jjc, bookmarks, opts.Stack, baseRev(opts))
	if err != nil {
		return nil, err
	}
	commits, err := jjc.Log(ctx, baseRev(opts)+".."+bookmark.ChangeID)
	if err != nil {
		return nil, err
	}
	owner, err := ghc.Owner(ctx)
	if err != nil {
		return nil, err
	}
	s := &Stack{Bookmark: bookmark.Name, HeadChangeID: bookmark.ChangeID, BaseBranch: base}
	for _, c := range commits {
		// Dash, not slash: git forbids refs/heads/<bookmark> and
		// refs/heads/<bookmark>/<id> from coexisting.
		branch := bookmark.Name + "-" + c.ChangeID
		e := Entry{Commit: c, Branch: branch}
		if !c.Empty {
			if e.PR, err = ghc.FindPR(ctx, owner, branch); err != nil {
				return nil, err
			}
		}
		s.Entries = append(s.Entries, e)
	}
	return s, nil
}

// chooseBookmark picks the stack bookmark. Per-commit tool bookmarks are named
// "<stack>-<change-id>", so they are recognized by that suffix and skipped.
func chooseBookmark(ctx context.Context, jjc *jj.Client, bookmarks []jj.Bookmark, want, base string) (jj.Bookmark, error) {
	if want != "" {
		for _, b := range bookmarks {
			if b.Name == want {
				return b, nil
			}
		}
		return jj.Bookmark{}, fmt.Errorf("bookmark %q not found", want)
	}
	var candidates []jj.Bookmark
	for _, b := range bookmarks {
		if strings.HasSuffix(b.Name, "-"+b.ChangeID) {
			continue // tool bookmark
		}
		// Exclude bookmarks at or below the base (e.g. main). A stack bookmark
		// may sit behind the base (just after a fetch), so descendants-of-base
		// is not required.
		ok, err := jjc.Match(ctx, b.ChangeID+" & ~::"+base)
		if err != nil {
			return jj.Bookmark{}, err
		}
		if ok {
			candidates = append(candidates, b)
		}
	}
	switch len(candidates) {
	case 0:
		return jj.Bookmark{}, fmt.Errorf("no stack bookmark found; create one on your top commit or pass --stack")
	case 1:
		return candidates[0], nil
	default:
		var names []string
		for _, b := range candidates {
			names = append(names, b.Name)
		}
		return jj.Bookmark{}, fmt.Errorf("multiple stack bookmarks found (%s); pick one with --stack", strings.Join(names, ", "))
	}
}
