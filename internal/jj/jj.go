// Package jj wraps the jj commands this tool needs.
package jj

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/levibostian/jj-spr/internal/runner"
)

// Client runs jj commands.
type Client struct {
	R runner.Runner
}

// Commit is the subset of a jj commit the tool cares about.
type Commit struct {
	ChangeID    string
	Description string
	Empty       bool
	// IDPrefix is the shortest change ID prefix that is unambiguous, as jj
	// accepts on the command line (change_id.shortest()).
	IDPrefix string
	// ShortID is the abbreviated change ID to display (change_id.shortest(8)).
	ShortID string
}

// Title is the first line of the commit description.
func (c Commit) Title() string {
	line, _, _ := strings.Cut(c.Description, "\n")
	return strings.TrimSpace(line)
}

// Body is the description after the first line.
func (c Commit) Body() string {
	_, rest, found := strings.Cut(c.Description, "\n")
	if !found {
		return ""
	}
	return strings.TrimSpace(rest)
}

// Bookmark is a local jj bookmark and the change it points to.
type Bookmark struct {
	Name     string
	ChangeID string
}

type wireCommit struct {
	ChangeID    string `json:"change_id"`
	Description string `json:"description"`
}

// Log returns the commits in revset, oldest first.
func (c *Client) Log(ctx context.Context, revset string) ([]Commit, error) {
	out, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"log", "--no-graph", "--reversed", "-r", revset, "-T", `json(self) ++ "\x1f" ++ if(empty, "1", "0") ++ "\x1f" ++ change_id.shortest() ++ "\x1f" ++ change_id.shortest(8) ++ "\n"`},
	})
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) != 4 {
			return nil, fmt.Errorf("unexpected jj log line: %q", line)
		}
		var w wireCommit
		if err := json.Unmarshal([]byte(parts[0]), &w); err != nil {
			return nil, fmt.Errorf("parsing jj log line: %w", err)
		}
		commits = append(commits, Commit{
			ChangeID:    w.ChangeID,
			Description: w.Description,
			Empty:       parts[1] == "1",
			IDPrefix:    parts[2],
			ShortID:     parts[3],
		})
	}
	return commits, nil
}

// TrunkBranch returns the branch name that the trunk() revset points at on
// remote. jj models git branches as bookmarks, so this is the remote
// bookmark's name. The first match wins; trunk() normally has exactly one.
func (c *Client) TrunkBranch(ctx context.Context, remote string) (string, error) {
	out, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		// Quoted so a remote name cannot inject template syntax.
		Args: []string{"log", "-r", "trunk()", "--no-graph", "-T",
			`remote_bookmarks.filter(|b| b.remote() == ` + strconv.Quote(remote) + `).map(|b| b.name()).join("\n") ++ "\n"`},
	})
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", fmt.Errorf("no remote bookmark at trunk() on remote %q", remote)
	}
	if i := strings.IndexByte(name, '\n'); i >= 0 {
		name = name[:i]
	}
	return name, nil
}

// Bookmarks returns all local bookmarks.
func (c *Client) Bookmarks(ctx context.Context) ([]Bookmark, error) {
	out, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"bookmark", "list", "-T", `name ++ "\t" ++ if(normal_target, normal_target.change_id(), "") ++ "\n"`},
	})
	if err != nil {
		return nil, err
	}
	var bookmarks []Bookmark
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		name, changeID, _ := strings.Cut(line, "\t")
		if changeID == "" {
			continue
		}
		bookmarks = append(bookmarks, Bookmark{Name: name, ChangeID: changeID})
	}
	return bookmarks, nil
}

// Workspace returns the name of the current jj workspace. The workspace whose
// target commit is @ is the current one.
func (c *Client) Workspace(ctx context.Context) (string, error) {
	atChangeID, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"log", "-r", "@", "--no-graph", "-T", "change_id"},
	})
	if err != nil {
		return "", err
	}
	out, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"workspace", "list", "-T", `name ++ "\t" ++ target.change_id() ++ "\n"`},
	})
	if err != nil {
		return "", err
	}
	atChangeID = strings.TrimSpace(atChangeID)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		name, id, _ := strings.Cut(line, "\t")
		if id == atChangeID {
			return name, nil
		}
	}
	return "", nil
}

// SetBookmark points a local bookmark at a change, creating it if needed.
func (c *Client) SetBookmark(ctx context.Context, name, changeID string) error {
	_, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"bookmark", "set", "--allow-backwards", "-r", changeID, name},
	})
	return err
}

// Push pushes the named bookmarks to the remote.
func (c *Client) Push(ctx context.Context, remote string, branches []string) error {
	if len(branches) == 0 {
		return nil
	}
	// Stack branches can contain an empty working-copy commit, so jj needs
	// this flag to push them.
	args := []string{"git", "push", "--remote", remote, "--allow-empty-description"}
	for _, b := range branches {
		args = append(args, "-b", b)
	}
	_, err := c.R.Run(ctx, runner.Command{Name: "jj", Args: args})
	return err
}

// Fetch fetches from the remote.
func (c *Client) Fetch(ctx context.Context, remote string) error {
	_, err := c.R.Run(ctx, runner.Command{Name: "jj", Args: []string{"git", "fetch", "--remote", remote}})
	return err
}

// Rebase reparents source (and its descendants) onto the given target.
func (c *Client) Rebase(ctx context.Context, source, onto string) error {
	_, err := c.R.Run(ctx, runner.Command{Name: "jj", Args: []string{"rebase", "-s", source, "-o", onto}})
	return err
}

// Match reports whether revset resolves to at least one commit.
func (c *Client) Match(ctx context.Context, revset string) (bool, error) {
	out, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"log", "--no-graph", "-r", revset, "-T", `change_id ++ "\n"`},
	})
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Conflicts returns the change IDs in revset that have conflicts.
func (c *Client) Conflicts(ctx context.Context, revset string) ([]string, error) {
	out, err := c.R.Run(ctx, runner.Command{
		Name: "jj",
		Args: []string{"log", "--no-graph", "-r", "conflicts() & (" + revset + ")", "-T", `change_id ++ "\n"`},
	})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ids = append(ids, line)
		}
	}
	return ids, nil
}
