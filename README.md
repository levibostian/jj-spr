# jj-spr

Each jj commit becomes a GitHub pull request. Stack them without juggling branches.

## Why

You already work in a linear series of jj commits. `jj-spr` turns that series
into a native GitHub stack, so you can:

- Stop creating one bookmark and one PR per commit by hand.
- Keep reviews small and ordered, one commit at a time.
- Rewrite, reorder, and squash freely — the same PR follows the same change.
- Merge in the GitHub UI, fetch, and resync the rest of the stack.

## Features

- **One bookmark, one stack.** A single jj bookmark marks the top of the stack.
  Everything from `trunk()` to that bookmark is published.
- **Commit description is the PR.** The first line becomes the PR title; the
  rest becomes the body. Edit locally and run `update` to push the change.
- **Stable PR identity.** Branches are named after jj change IDs, so a rewritten
  commit updates its existing PR instead of creating a new one.
- **Native GitHub stacks.** Uses `gh stack link`; no merge bot, no extra service.
- **Three commands.** `update`, `status`, `sync`. Nothing else.

## Prerequisites

- [jj](https://jj-vcs.dev) (recent version)
- [GitHub CLI](https://cli.github.com) plus the `gh-stack` extension:
  `gh extension install github/gh-stack`
- A jj repo with a GitHub `origin`. It runs from any jj workspace, colocated
  or not — no `.git` checkout required.

## Quick start

```sh
# Create some commits and a bookmark on the top one.
jj new main -m "Add model"
jj new -m "Add API"
jj bookmark create mystack -r @-

# Publish. Each commit becomes a PR, stacked in order.
jj-spr update
```

## Usage

```sh
jj-spr update   # create or update PRs and the GitHub stack
jj-spr status   # show the stack and per-PR status
jj-spr sync     # fetch, rebase onto trunk, then update
```

`update` never moves history. Rewrite commits with `jj`, then run it again; the
same PRs are updated in place.

After merging PRs in the GitHub UI (or through a stack merge), run:

```sh
jj git fetch
jj-spr sync
```

`sync` rebases the unmerged commits onto the new trunk and republishes the
remaining PRs.

Example `status`:

```
mystack  (base: main)
  ➖➖❌❌ #43     zmylxtxx Add UI
  ⌛✅✅✅ #42     rlvkypqw Add API
  ✅✅✅✅ #41     qpvuntsm Add model
   │ │ │ └─ stack: all PRs below are ready
   │ │ └─── conflicts: no merge conflicts
   │ └───── approval: PR is approved
   └─────── checks: CI checks pass
```

The first column after the PR number is the change ID, abbreviated to the
shortest unambiguous prefix (shown in color on a terminal) followed by the
first line of the commit description.

## Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--stack <name>` | auto-detect | Bookmark that names the stack. |
| `--base <branch>` | `trunk()` | Base branch/bookmark of the bottom PR and of the jj commit range. |
| `--remote <name>` | `origin` | Remote to fetch/push. |
| `--draft` | off | Create new PRs as drafts. |
| `-r, --reviewer <handle>` | none | Request reviews from these users/teams on newly created PRs. Repeatable; accepts comma-separated values. |
| `--json` | off | `status`: machine-readable output. |
| `-v` | off | Log every `jj`/`gh` command. |

## Configuration

`jj-spr` reads the same `.spr.yml` as [`spr`](https://github.com/ejoffe/spr),
so one file configures both tools. Put it in the root of your jj workspace.
Only `defaultReviewers` is used today:

```yaml
# .spr.yml
defaultReviewers:
  - teammate
  - acme/team
```

Reviewers are requested on newly created PRs only. `--reviewer` adds to this
list for a single run; it does not change the file. A malformed file stops the
run with an error; a missing file just uses defaults.

## Development

```sh
go build ./...
go test ./...
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for architecture and testing notes.
