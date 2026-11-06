// Package cli wires the three commands to the stack logic.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/levibostian/jj-spr/internal/gh"
	"github.com/levibostian/jj-spr/internal/jj"
	"github.com/levibostian/jj-spr/internal/runner"
	"github.com/levibostian/jj-spr/internal/stack"
)

// App is the command-line application. Fields are swapped in tests.
type App struct {
	Runner runner.Runner
	Stdout io.Writer
	Stderr io.Writer
}

// New returns an App that runs real commands.
func New() *App {
	return &App{Runner: runner.Exec{}, Stdout: os.Stdout, Stderr: os.Stderr}
}

type flags struct {
	opts    stack.Options
	json    bool
	verbose bool
}

// Run dispatches a command and returns the process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.usage()
		return 1
	}
	switch args[0] {
	case "update":
		return a.update(ctx, args[1:])
	case "status":
		return a.status(ctx, args[1:])
	case "sync":
		return a.sync(ctx, args[1:])
	case "help", "-h", "--help":
		a.usage()
		return 0
	case "version", "--version":
		fmt.Fprintln(a.Stdout, "0.1.0")
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown command %q\n\n", args[0])
		a.usage()
		return 1
	}
}

func (a *App) usage() {
	fmt.Fprint(a.Stdout, `jj-spr: stacked GitHub PRs from jj commits

Usage:
  jj-spr update   create/update PRs and the GitHub stack
  jj-spr status   show the stack and per-PR status
  jj-spr sync     fetch, rebase onto trunk, then update

Flags: --stack <bookmark>  --base <branch>  --remote <name>  --draft  --json  -v
`)
}

func (a *App) parse(name string, args []string) (flags, error) {
	var f flags
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&f.opts.Stack, "stack", "", "stack bookmark (default: auto-detect)")
	fs.StringVar(&f.opts.Base, "base", "", "base branch or bookmark (default: jj trunk())")
	fs.StringVar(&f.opts.Remote, "remote", "origin", "remote name")
	fs.BoolVar(&f.opts.Draft, "draft", false, "create new PRs as drafts")
	fs.BoolVar(&f.json, "json", false, "status: output JSON")
	fs.BoolVar(&f.verbose, "v", false, "log jj/gh commands")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return f, nil
}

// clients builds the jj and gh clients. It resolves owner/repo from the jj
// remote once and pins it on the gh client as GH_REPO, so gh works without a
// .git checkout.
func (a *App) clients(ctx context.Context, f flags) (*jj.Client, *gh.Client, error) {
	r := a.Runner
	if f.verbose {
		r = runner.Verbose{Inner: r, Writer: a.Stdout}
	}
	jjc := &jj.Client{R: r}
	ghc := &gh.Client{R: r}
	remoteURL, err := jjc.RemoteURL(ctx, f.opts.Remote)
	if err != nil {
		return nil, nil, err
	}
	repo, err := gh.ParseRemote(remoteURL)
	if err != nil {
		return nil, nil, err
	}
	ghc.Repo = repo
	return jjc, ghc, nil
}

func (a *App) update(ctx context.Context, args []string) int {
	f, err := a.parse("update", args)
	if err != nil {
		return 2
	}
	jjc, ghc, err := a.clients(ctx, f)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	if err := ghc.Available(ctx); err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	res, err := stack.Update(ctx, jjc, ghc, f.opts)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	a.printResult(res, ghc.Repo)
	return 0
}

func (a *App) sync(ctx context.Context, args []string) int {
	f, err := a.parse("sync", args)
	if err != nil {
		return 2
	}
	jjc, ghc, err := a.clients(ctx, f)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	if err := ghc.Available(ctx); err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	res, err := stack.Sync(ctx, jjc, ghc, f.opts)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	a.printResult(res, ghc.Repo)
	return 0
}

func (a *App) status(ctx context.Context, args []string) int {
	f, err := a.parse("status", args)
	if err != nil {
		return 2
	}
	jjc, ghc, err := a.clients(ctx, f)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	report, err := stack.Status(ctx, jjc, ghc, f.opts)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	if f.json {
		b, err := report.JSON()
		if err != nil {
			fmt.Fprintln(a.Stderr, "error:", err)
			return 1
		}
		fmt.Fprintln(a.Stdout, string(b))
		return 0
	}
	stack.Render(a.Stdout, report)
	return 0
}

// printResult reports what a publish run changed, as clickable GitHub URLs.
// The native stack has no web URL of its own, so it is shown as the ordered
// member PRs (bottom to top).
func (a *App) printResult(res *stack.Result, repo gh.Repo) {
	lines := 0
	printURLs := func(numbers []int) {
		for _, n := range numbers {
			fmt.Fprintf(a.Stdout, "  %s\n", repo.PRURL(n))
		}
	}
	printPRs := func(label string, numbers []int) {
		if len(numbers) == 0 {
			return
		}
		fmt.Fprintf(a.Stdout, "%s:\n", label)
		printURLs(numbers)
		lines++
	}
	printPRs("created", res.Created)
	printPRs("updated", res.Updated)
	printPRs("retired (merged/closed)", res.Retired)
	if res.StackNumber > 0 {
		fmt.Fprintf(a.Stdout, "stack: #%d (bottom to top)\n", res.StackNumber)
		printURLs(res.StackPRs)
		lines++
	}
	if lines == 0 {
		fmt.Fprintln(a.Stdout, "already up to date")
	}
}
