// Package gh wraps the gh commands this tool needs.
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/levibostian/jj-spr/internal/runner"
)

// Client runs gh commands.
type Client struct {
	R runner.Runner
}

// PR is a pull request, from either the pulls list API or `gh pr view`.
type PR struct {
	Number           int     `json:"number"`
	State            string  `json:"state"`
	MergedAt         *string `json:"merged_at"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	IsDraft          bool    `json:"isDraft"`
	URL              string  `json:"url"`
	HeadRefName      string  `json:"headRefName"`
	BaseRefName      string  `json:"baseRefName"`
	Mergeable        string  `json:"mergeable"`
	MergeStateStatus string  `json:"mergeStateStatus"`
	ReviewDecision   string  `json:"reviewDecision"`
	Checks           []Check `json:"statusCheckRollup"`
}

// Merged reports whether the PR has been merged.
func (p PR) Merged() bool { return p.MergedAt != nil && *p.MergedAt != "" }

// Retired reports whether the PR should be left out of the published stack.
func (p PR) Retired() bool { return p.Merged() || strings.EqualFold(p.State, "closed") }

// Check is one CI check from statusCheckRollup.
type Check struct {
	State      string `json:"state"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// Owner returns the repository owner login.
func (c *Client) Owner(ctx context.Context) (string, error) {
	return c.jq(ctx, "repo", "view", "--json", "owner", "--jq", ".owner.login")
}

// DefaultBranch returns the repository default branch name.
func (c *Client) DefaultBranch(ctx context.Context) (string, error) {
	return c.jq(ctx, "repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
}

func (c *Client) jq(ctx context.Context, args ...string) (string, error) {
	out, err := c.R.Run(ctx, runner.Command{Name: "gh", Args: args})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Available reports whether the gh-stack extension is installed.
func (c *Client) Available(ctx context.Context) error {
	_, err := c.R.Run(ctx, runner.Command{Name: "gh", Args: []string{"stack", "--help"}})
	if err != nil {
		return fmt.Errorf("gh stack is not available: install it with `gh extension install github/gh-stack` (%w)", err)
	}
	return nil
}

// FindPR returns the most recent PR whose head is owner/branch, or nil.
func (c *Client) FindPR(ctx context.Context, owner, branch string) (*PR, error) {
	out, err := c.R.Run(ctx, runner.Command{Name: "gh", Args: []string{
		"api", "--method", "GET", "repos/{owner}/{repo}/pulls",
		"-f", "head=" + owner + ":" + branch,
		"-f", "state=all",
		"-f", "per_page=100",
	}})
	if err != nil {
		return nil, err
	}
	var wire []struct {
		Number   int     `json:"number"`
		State    string  `json:"state"`
		MergedAt *string `json:"merged_at"`
		Draft    bool    `json:"draft"`
		Title    string  `json:"title"`
		Body     string  `json:"body"`
		HTMLURL  string  `json:"html_url"`
		Head     struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := json.Unmarshal([]byte(out), &wire); err != nil {
		return nil, fmt.Errorf("parsing pulls: %w", err)
	}
	if len(wire) == 0 {
		return nil, nil
	}
	p := wire[0]
	return &PR{
		Number:      p.Number,
		State:       p.State,
		MergedAt:    p.MergedAt,
		Title:       p.Title,
		Body:        p.Body,
		IsDraft:     p.Draft,
		URL:         p.HTMLURL,
		HeadRefName: p.Head.Ref,
		BaseRefName: p.Base.Ref,
	}, nil
}

// CreatePR opens a PR and returns its number.
func (c *Client) CreatePR(ctx context.Context, head, base, title, body string, draft bool) (int, error) {
	args := []string{"pr", "create", "--head", head, "--base", base, "--title", title, "--body-file", "-"}
	if draft {
		args = append(args, "--draft")
	}
	out, err := c.R.Run(ctx, runner.Command{Name: "gh", Args: args, Stdin: body})
	if err != nil {
		return 0, err
	}
	url := strings.TrimSpace(out)
	idx := strings.LastIndex(url, "/pull/")
	if idx < 0 {
		return 0, fmt.Errorf("could not find PR number in %q", url)
	}
	return strconv.Atoi(url[idx+len("/pull/"):])
}

// EditPR updates a PR's title and body.
func (c *Client) EditPR(ctx context.Context, number int, title, body string) error {
	_, err := c.R.Run(ctx, runner.Command{
		Name: "gh",
		Args: []string{"pr", "edit", strconv.Itoa(number), "--title", title, "--body-file", "-"},
		Stdin: body,
	})
	return err
}

// EditPRBase changes a PR's base branch.
func (c *Client) EditPRBase(ctx context.Context, number int, base string) error {
	_, err := c.jq(ctx, "pr", "edit", strconv.Itoa(number), "--base", base)
	return err
}

// ViewPR returns the PR for a head branch with status fields.
func (c *Client) ViewPR(ctx context.Context, branch string) (*PR, error) {
	out, err := c.R.Run(ctx, runner.Command{Name: "gh", Args: []string{
		"pr", "view", branch, "--json",
		"number,title,state,isDraft,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup,url,baseRefName",
	}})
	if err != nil {
		return nil, err
	}
	var p PR
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		return nil, fmt.Errorf("parsing PR: %w", err)
	}
	return &p, nil
}

// RemoteStack is the stack returned by the Stacks REST API.
type RemoteStack struct {
	Number       int `json:"number"`
	PullRequests []struct {
		Number   int     `json:"number"`
		State    string  `json:"state"`
		MergedAt *string `json:"merged_at"`
	} `json:"pull_requests"`
}

// OpenPRs returns the open PR numbers in the stack, bottom to top.
func (s *RemoteStack) OpenPRs() []int {
	var out []int
	for _, p := range s.PullRequests {
		if strings.EqualFold(p.State, "open") && (p.MergedAt == nil || *p.MergedAt == "") {
			out = append(out, p.Number)
		}
	}
	return out
}

// HasMerged reports whether any PR in the stack is merged.
func (s *RemoteStack) HasMerged() bool {
	for _, p := range s.PullRequests {
		if p.MergedAt != nil && *p.MergedAt != "" {
			return true
		}
	}
	return false
}

// FindStackForPR returns the stack containing pr, or nil.
func (c *Client) FindStackForPR(ctx context.Context, pr int) (*RemoteStack, error) {
	out, err := c.R.Run(ctx, runner.Command{Name: "gh", Args: []string{
		"api", "repos/{owner}/{repo}/stacks?pull_request=" + strconv.Itoa(pr),
	}})
	if err != nil {
		return nil, err
	}
	var stacks []RemoteStack
	if err := json.Unmarshal([]byte(out), &stacks); err != nil {
		return nil, fmt.Errorf("parsing stacks: %w", err)
	}
	if len(stacks) == 0 {
		return nil, nil
	}
	return &stacks[0], nil
}

// Unstack removes eligible PRs from a stack on GitHub.
func (c *Client) Unstack(ctx context.Context, number int) error {
	_, err := c.jq(ctx, "api", "--method", "POST", "repos/{owner}/{repo}/stacks/"+strconv.Itoa(number)+"/unstack")
	return err
}

// StackLink creates or updates a native stack from PR numbers, bottom to top.
func (c *Client) StackLink(ctx context.Context, base string, prs []int) error {
	args := []string{"stack", "link", "--base", base}
	for _, n := range prs {
		args = append(args, strconv.Itoa(n))
	}
	_, err := c.jq(ctx, args...)
	return err
}
