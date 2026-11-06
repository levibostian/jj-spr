// Package stack resolves a jj bookmark into a GitHub stack and publishes it.
package stack

import (
	"context"
	"fmt"

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
	Stack     string
	Base      string
	Remote    string
	Draft     bool
	Reviewers []string
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

// Resolve builds the stack for a bookmark. When Stack is empty the bookmark
// nearest @ above base is chosen (see jj.Client.StackBookmark).
func Resolve(ctx context.Context, jjc *jj.Client, ghc *gh.Client, opts Options) (*Stack, error) {
	base := opts.Base
	if base == "" {
		var err error
		if base, err = jjc.TrunkBranch(ctx, opts.remote()); err != nil {
			return nil, err
		}
	}
	bookmark, err := chooseBookmark(ctx, jjc, opts.Stack, baseRev(opts))
	if err != nil {
		return nil, err
	}
	commits, err := jjc.Log(ctx, baseRev(opts)+".."+bookmark.ChangeID)
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
			if e.PR, err = ghc.FindPR(ctx, ghc.Repo.Owner, branch); err != nil {
				return nil, err
			}
		}
		s.Entries = append(s.Entries, e)
	}
	return s, nil
}

// chooseBookmark picks the stack bookmark. With --stack it is the named
// bookmark; otherwise it is the current workspace's stack (jjc.StackBookmark).
func chooseBookmark(ctx context.Context, jjc *jj.Client, want, base string) (jj.Bookmark, error) {
	if want == "" {
		return jjc.StackBookmark(ctx, base)
	}
	bookmarks, err := jjc.Bookmarks(ctx)
	if err != nil {
		return jj.Bookmark{}, err
	}
	for _, b := range bookmarks {
		if b.Name == want {
			return b, nil
		}
	}
	return jj.Bookmark{}, fmt.Errorf("bookmark %q not found", want)
}
