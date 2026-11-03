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

func (a *App) clients(f flags) (*jj.Client, *gh.Client) {
	r := a.Runner
	if f.verbose {
		r = runner.Verbose{Inner: r, Writer: a.Stdout}
	}
	return &jj.Client{R: r}, &gh.Client{R: r}
}

func (a *App) update(ctx context.Context, args []string) int {
	f, err := a.parse("update", args)
	if err != nil {
		return 2
	}
	jjc, ghc := a.clients(f)
	if err := ghc.Available(ctx); err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	res, err := stack.Update(ctx, jjc, ghc, f.opts)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	a.printResult(res)
	return 0
}

func (a *App) sync(ctx context.Context, args []string) int {
	f, err := a.parse("sync", args)
	if err != nil {
		return 2
	}
	jjc, ghc := a.clients(f)
	if err := ghc.Available(ctx); err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	res, err := stack.Sync(ctx, jjc, ghc, f.opts)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error:", err)
		return 1
	}
	a.printResult(res)
	return 0
}

func (a *App) status(ctx context.Context, args []string) int {
	f, err := a.parse("status", args)
	if err != nil {
		return 2
	}
	jjc, ghc := a.clients(f)
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

func (a *App) printResult(res *stack.Result) {
	lines := 0
	if len(res.Created) > 0 {
		fmt.Fprintf(a.Stdout, "created: %v\n", res.Created)
		lines++
	}
	if len(res.Updated) > 0 {
		fmt.Fprintf(a.Stdout, "updated: %v\n", res.Updated)
		lines++
	}
	if len(res.Retired) > 0 {
		fmt.Fprintf(a.Stdout, "retired (merged/closed): %v\n", res.Retired)
		lines++
	}
	if res.StackNumber > 0 {
		fmt.Fprintf(a.Stdout, "stack: #%d\n", res.StackNumber)
		lines++
	}
	if lines == 0 {
		fmt.Fprintln(a.Stdout, "already up to date")
	}
}
