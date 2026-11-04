# Development

## Architecture

The tool is a thin orchestrator over the `jj` and `gh` CLIs.

- `main.go` → `internal/cli`. Parses flags and dispatches `update`/`status`/`sync`.
- `internal/runner`: `Runner` interface over external commands. `Exec` runs real
  processes; `Fake` scripts responses in unit tests; `Verbose` logs commands.
- `internal/jj`: typed wrappers for the jj commands used (log, bookmark, push,
  fetch, rebase, conflicts).
- `internal/gh`: typed wrappers for gh commands (PR lookup/create/edit, stack
  create/link/unstack). Resolves `owner/repo` from the remote and pins it on
  every call as `GH_REPO`.
- `internal/stack`: domain logic. `Resolve` builds a `Stack` from a bookmark,
  `publish` pushes and reconciles PRs, `Sync` fetches/rebases, `BuildReport`
  renders status.

Everything external goes through `runner.Runner`, so tests inject a fake or
mock the real binaries.

## Key design decisions

- **Branches are `<bookmark>-<change-id>`.** jj change IDs survive rewrites and
  reorders, so the same GitHub PR follows the same logical change. A local
  bookmark of the same name persists per commit and is recognized by its
  `-<change-id>` suffix during stack auto-detection. A dash, not a slash: git
  forbids `refs/heads/<bookmark>` and `refs/heads/<bookmark>/<change-id>`
  coexisting, and the user's bookmark is also exported as a git ref.
- **Stack auto-detection walks out from `@`, then resolves the stack top.**
  `jj.StackBookmark` finds the bookmark nearest `@` — ancestors-or-self first
  (`heads(bookmarks() & ::@ & ~::trunk())`), then strict descendants
  (`roots(bookmarks() & @:: & ~::@ & ~::trunk())`). That nearest bookmark only
  names the stack; the top is then resolved to the topmost bookmarked commit
  carrying that name (a user bookmark, or a per-commit tool bookmark
  `<stack>-<change-id>`). So `@` can sit mid-stack and the whole
  `base..bookmark` stack is still published, unrelated stacks elsewhere in the
  repo are never candidates, and detection works before a user bookmark is
  moved onto `@`. Distinct stack names on the nearest commit are reported as an
  ambiguity for `--stack`.
- **Push allows empty descriptions.** Stack branches can contain an empty
  working-copy commit, so `jj git push` runs with `--allow-empty-description`.
- **`gh stack link` is append-only.** It cannot reorder or remove PRs, and it
  refuses stacks that list merged PRs. `stack.Reconcile` reads the current
  stack and, when the desired order is not a pure append, unstacks (via the
  Stacks REST API) and re-links.
- **No `.git` checkout required.** The tool must run from a jj workspace that
  is not colocated, so nothing reads `.git`. It reads `owner/repo` from the jj
  remote once (`jj git remote list`) and passes `GH_REPO=owner/repo` to every
  gh call (`runner.Command.Env`); gh otherwise discovers the repository by
  shelling out to git. Never add a gh command that relies on the checkout.
- **No local state file.** PRs are found by head branch via the pulls REST API.

## Testing

```sh
go test ./...
go test -coverpkg=./... -coverprofile=cover.out ./... && go tool cover -func=cover.out
```

- Unit tests use `runner.Fake` with scripted `jj`/`gh` output.
- `internal/cli/integration_test.go` runs the real `exec` runner against mocked
  binaries injected into `PATH` with
  [`mock-a-bin-go`](https://github.com/levibostian/mock-a-bin-go).

Tests never touch a real jj or GitHub repository.
