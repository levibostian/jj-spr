package stack

import (
	"context"
	"fmt"
	"strings"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
)

// Status resolves the stack and fetches live PR detail.
func Status(ctx context.Context, jjc *jj.Client, ghc *gh.Client, opts Options) (*Report, error) {
	st, err := Resolve(ctx, jjc, ghc, opts)
	if err != nil {
		return nil, err
	}
	return BuildReport(ctx, ghc, st)
}

// Sync fetches, rebases the active stack onto trunk, checks for conflicts, and
// publishes.
func Sync(ctx context.Context, jjc *jj.Client, ghc *gh.Client, opts Options) (*Result, error) {
	if err := jjc.Fetch(ctx, opts.remote()); err != nil {
		return nil, err
	}
	st, err := Resolve(ctx, jjc, ghc, opts)
	if err != nil {
		return nil, err
	}
	if active := st.Active(); len(active) > 0 {
		if err := jjc.Rebase(ctx, active[0].Commit.ChangeID, "trunk()"); err != nil {
			return nil, err
		}
	}
	conflicts, err := jjc.Conflicts(ctx, "trunk().."+st.HeadChangeID)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("conflicts in %s; resolve them and run sync again", strings.Join(conflicts, ", "))
	}
	st, err = Resolve(ctx, jjc, ghc, opts)
	if err != nil {
		return nil, err
	}
	return publish(ctx, jjc, ghc, st, opts)
}
