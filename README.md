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
- A colocated jj/git repo with a GitHub `origin`.

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
  ✅✅✅✅ #41 Add model   mystack-qpvuntsm
  ✅⏳✅✅ #42 Add API     mystack-rlvkypqw
  ➖➖📝❌ #43 Add UI      mystack-zmylxtxx
```

## Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--stack <name>` | auto-detect | Bookmark that names the stack. |
| `--base <branch>` | repo default branch | Base branch of the bottom PR. |
| `--remote <name>` | `origin` | Remote to fetch/push. |
| `--draft` | off | Create new PRs as drafts. |
| `--json` | off | `status`: machine-readable output. |
| `-v` | off | Log every `jj`/`gh` command. |

## Development

```sh
go build ./...
go test ./...
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for architecture and testing notes.
