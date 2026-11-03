package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
)

// Bits are the four at-a-glance status signals for one PR.
type Bits struct {
	Checks string `json:"checks"`
	Review string `json:"review"`
	Merge  string `json:"merge"`
	Stack  string `json:"stack"`
}

// StatusRow is one entry plus its live PR detail.
type StatusRow struct {
	Entry  Entry  `json:"entry"`
	Detail *gh.PR `json:"detail,omitempty"`
	Bits   Bits   `json:"bits"`
}

// Report is the rendered status of a stack.
type Report struct {
	Stack *Stack      `json:"stack"`
	Rows  []StatusRow `json:"rows"`
}

// BuildReport fetches live detail for every active PR.
func BuildReport(ctx context.Context, ghc *gh.Client, st *Stack) (*Report, error) {
	r := &Report{Stack: st}
	for _, e := range st.Entries {
		row := StatusRow{Entry: e}
		if e.PR != nil {
			row.Detail = e.PR
			if !e.Retired() {
				detail, err := ghc.ViewPR(ctx, e.Branch)
				if err != nil {
					return nil, err
				}
				row.Detail = detail
			}
		}
		row.Bits = bitsFor(row.Detail)
		r.Rows = append(r.Rows, row)
	}
	for i := range r.Rows {
		if allGreen(r.Rows[:i]) {
			r.Rows[i].Bits.Stack = "clear"
		} else {
			r.Rows[i].Bits.Stack = "blocked"
		}
	}
	return r, nil
}

func allGreen(rows []StatusRow) bool {
	for _, row := range rows {
		if !row.Entry.Active() {
			continue
		}
		if row.Bits.Checks != "pass" && row.Bits.Checks != "none" {
			return false
		}
		if row.Bits.Merge != "clean" {
			return false
		}
	}
	return true
}

func bitsFor(p *gh.PR) Bits {
	if p == nil {
		return Bits{Checks: "none", Review: "none", Merge: "none", Stack: "none"}
	}
	return Bits{Checks: checksBit(p), Review: reviewBit(p), Merge: mergeBit(p)}
}

func checksBit(p *gh.PR) string {
	if len(p.Checks) == 0 {
		return "none"
	}
	bit := "pass"
	for _, c := range p.Checks {
		switch strings.ToUpper(c.State + c.Conclusion) {
		case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTIONREQUIRED":
			return "fail"
		case "PENDING", "IN_PROGRESS", "QUEUED", "EXPECTED", "":
			bit = "pending"
		}
	}
	return bit
}

func reviewBit(p *gh.PR) string {
	switch strings.ToUpper(p.ReviewDecision) {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes"
	case "REVIEW_REQUIRED":
		return "required"
	default:
		return "none"
	}
}

func mergeBit(p *gh.PR) string {
	switch strings.ToUpper(p.MergeStateStatus) {
	case "CLEAN":
		return "clean"
	case "DIRTY":
		return "conflicts"
	case "BLOCKED":
		return "blocked"
	case "DRAFT":
		return "draft"
	case "BEHIND":
		return "behind"
	case "UNSTABLE":
		return "unstable"
	default:
		return "unknown"
	}
}

// Render writes a human-readable status table.
func Render(w io.Writer, r *Report) {
	color := colorEnabled(w)
	fmt.Fprintf(w, "%s  (base: %s)\n", r.Stack.Bookmark, r.Stack.BaseBranch)
	for _, row := range r.Rows {
		label := "no PR"
		if row.Detail != nil {
			label = fmt.Sprintf("#%d", row.Detail.Number)
		} else if row.Entry.Commit.Empty {
			label = "(empty)"
		}
		fmt.Fprintf(w, "  %s %-7s %s %s\n",
			row.Bits.String(), label, shortID(row.Entry.Commit, color), row.Entry.Commit.Title())
	}
}

// shortID renders the abbreviated change ID, highlighting the shortest
// unambiguous prefix with jj's own change-ID colors: bright for the prefix,
// dim for the rest.
func shortID(c jj.Commit, color bool) string {
	if c.ShortID == "" {
		return c.ChangeID
	}
	prefixLen := len(c.IDPrefix)
	if !color || prefixLen == 0 {
		return c.ShortID
	}
	if prefixLen > len(c.ShortID) {
		prefixLen = len(c.ShortID)
	}
	return "\x1b[1m\x1b[38;5;5m" + c.ShortID[:prefixLen] + "\x1b[0m" +
		"\x1b[38;5;8m" + c.ShortID[prefixLen:] + "\x1b[39m"
}

// colorEnabled reports whether ANSI color should be written to w. Color is off
// when the caller opts out (NO_COLOR, dumb terminal) or w is not a terminal, so
// piped and redirected output stays plain text.
func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// String renders the four bits as emoji.
func (b Bits) String() string {
	return emoji(b.Checks) + emoji(b.Review) + emoji(b.Merge) + emoji(b.Stack)
}

func emoji(kind string) string {
	switch kind {
	case "pass", "approved", "clean", "clear":
		return "✅"
	case "fail", "changes", "conflicts", "blocked":
		return "❌"
	case "pending", "required", "behind":
		return "⏳"
	case "draft":
		return "📝"
	case "unstable":
		return "⚠️"
	case "none":
		return "➖"
	default:
		return "❔"
	}
}

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
