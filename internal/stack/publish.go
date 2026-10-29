package stack

import (
	"context"
	"strings"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
)

// Result summarizes what a publish run did.
type Result struct {
	Created     []int
	Updated     []int
	Retired     []int
	StackNumber int
}

// Update resolves the stack and publishes it.
func Update(ctx context.Context, jjc *jj.Client, ghc *gh.Client, opts Options) (*Result, error) {
	st, err := Resolve(ctx, jjc, ghc, opts)
	if err != nil {
		return nil, err
	}
	return publish(ctx, jjc, ghc, st, opts)
}

func publish(ctx context.Context, jjc *jj.Client, ghc *gh.Client, st *Stack, opts Options) (*Result, error) {
	res := &Result{}
	active := st.Active()

	var branches []string
	for _, e := range st.Entries {
		if e.Retired() && e.PR != nil {
			res.Retired = append(res.Retired, e.PR.Number)
		}
	}
	for _, e := range active {
		if err := jjc.SetBookmark(ctx, e.Branch, e.Commit.ChangeID); err != nil {
			return nil, err
		}
		branches = append(branches, e.Branch)
	}
	if err := jjc.Push(ctx, opts.remote(), branches); err != nil {
		return nil, err
	}

	prs := make([]int, 0, len(active))
	for i, e := range active {
		base := st.BaseBranch
		if i > 0 {
			base = active[i-1].Branch
		}
		number, err := ensurePR(ctx, ghc, e, base, opts.Draft, res)
		if err != nil {
			return nil, err
		}
		prs = append(prs, number)
	}

	stackNumber, err := Reconcile(ctx, ghc, st.BaseBranch, prs)
	if err != nil {
		return nil, err
	}
	res.StackNumber = stackNumber
	return res, nil
}

func ensurePR(ctx context.Context, ghc *gh.Client, e Entry, base string, draft bool, res *Result) (int, error) {
	title, body := e.Commit.Title(), e.Commit.Body()
	if title == "" {
		title = string(e.Commit.ChangeID)
	}
	if e.PR == nil {
		number, err := ghc.CreatePR(ctx, e.Branch, base, title, body, draft)
		if err != nil {
			return 0, err
		}
		res.Created = append(res.Created, number)
		return number, nil
	}
	// Skip gh pr edit when nothing changed to avoid review-notification noise.
	if strings.TrimSpace(e.PR.Title) != title || strings.TrimSpace(e.PR.Body) != body {
		if err := ghc.EditPR(ctx, e.PR.Number, title, body); err != nil {
			return 0, err
		}
		res.Updated = append(res.Updated, e.PR.Number)
	}
	if e.PR.BaseRefName != base {
		if err := ghc.EditPRBase(ctx, e.PR.Number, base); err != nil {
			return 0, err
		}
	}
	return e.PR.Number, nil
}

// Reconcile makes the native GitHub stack match prs (bottom to top).
func Reconcile(ctx context.Context, ghc *gh.Client, base string, prs []int) (int, error) {
	if len(prs) == 0 {
		return 0, nil
	}
	if len(prs) == 1 {
		current, err := ghc.FindStackForPR(ctx, prs[0])
		if err != nil {
			return 0, err
		}
		if current != nil {
			if err := ghc.Unstack(ctx, current.Number); err != nil {
				return 0, err
			}
		}
		return 0, ghc.EditPRBase(ctx, prs[0], base)
	}

	current, err := ghc.FindStackForPR(ctx, prs[0])
	if err != nil {
		return 0, err
	}
	if current != nil && (current.HasMerged() || !isOrderedPrefix(current.OpenPRs(), prs)) {
		if err := ghc.Unstack(ctx, current.Number); err != nil {
			return 0, err
		}
	}
	if err := ghc.StackLink(ctx, base, prs); err != nil {
		return 0, err
	}
	after, err := ghc.FindStackForPR(ctx, prs[0])
	if err != nil || after == nil {
		return 0, err
	}
	return after.Number, nil
}

// isOrderedPrefix reports whether prefix is a prefix of full, in order.
func isOrderedPrefix(prefix, full []int) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i, n := range prefix {
		if full[i] != n {
			return false
		}
	}
	return true
}
