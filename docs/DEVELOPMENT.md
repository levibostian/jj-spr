# Development

## Architecture

The tool is a thin orchestrator over the `jj` and `gh` CLIs.

- `main.go` → `internal/cli`. Parses flags and dispatches `update`/`status`/`sync`.
- `internal/runner`: `Runner` interface over external commands. `Exec` runs real
  processes; `Fake` scripts responses in unit tests; `Verbose` logs commands.
- `internal/jj`: typed wrappers for the jj commands used (log, bookmark, push,
  fetch, rebase, conflicts).
- `internal/gh`: typed wrappers for gh commands (PR lookup/create/edit, stack
  create/link/unstack).
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
- **Stack auto-detection accepts bookmarks behind trunk.** `chooseBookmark`
  keeps non-tool bookmarks that are not ancestors of `trunk()` (`<C> & ~::trunk()`),
  so `sync` still finds the stack after a fetch moves trunk ahead.
- **Push allows empty descriptions.** Stack branches can contain an empty
  working-copy commit, so `jj git push` runs with `--allow-empty-description`.
- **`gh stack link` is append-only.** It cannot reorder or remove PRs, and it
  refuses stacks that list merged PRs. `stack.Reconcile` reads the current
  stack and, when the desired order is not a pure append, unstacks (via the
  Stacks REST API) and re-links.
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
