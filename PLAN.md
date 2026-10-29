# jj-spr — Implementation Plan

A small Go CLI that turns a linear set of Jujutsu (jj) commits into a native
GitHub stacked pull request, and keeps that stack in sync. It is a jj-native
replacement for the `update`, `status`, and `sync` commands of
[`spr`](https://github.com/ejoffe/spr), built on top of GitHub's native
stacked PRs and the `gh` CLI.

---

## 1. Purpose

One user bookmark labels one stack. Every commit from `trunk()` up to (and
including) that bookmark is one PR. Local commit descriptions are the PR
title/body. The tool materializes throwaway jj bookmarks (and their remote
branches) per commit, creates/updates the PRs with `gh`, and links them into a
native GitHub stack with `gh stack link`.

## 2. Goals

- Exactly three commands: `update`, `status`, `sync`. No merge, no config
  wizards, no interactive TUI.
- As little custom logic as possible. Push PR/stack/API work down to `gh`
  (`gh pr create`, `gh pr edit`, `gh pr view`, `gh api`, `gh stack link`) and
  history work down to `jj`.
- Only `gh stack link` is used from the `gh stack` command family.
- One user bookmark defines a stack. The user never creates per-commit
  branches.
- Commit description ↔ PR title/body, updated on every `update`.
- Stable PR identity across jj rewrites (`jj` change IDs are the join key).
- Handle the post-merge case: the user merges in the GitHub UI, runs
  `jj git fetch` and rebases, then runs this tool. Expected result after
  `jj-spr sync`: the merged commits drop out of the published stack, the
  remaining commits are rebased onto `trunk()`, and the remaining PRs keep
  their numbers while their bases/diffs show only their own changes. See the
  example in §5 (`Post-merge example`).

## 3. Non-goals

- No merging (merge happens in the GitHub UI).
- No local stack tracking file / state database.
- No `gh stack` subcommands other than `link` (`submit`, `sync`, `init`,
  `modify`, `unstack`, … are not called).
- No support for non-linear stacks, multiple bookmarks in one stack, or
  cross-repo PRs.
- No non-colocated jj repositories (see §13).

## 4. Environment & prerequisites

Assume `jj`, `gh`, and the `gh-stack` extension are already installed and
authenticated. The tool does one small up-front check — that `gh stack` is
available — and otherwise just prints a short pointer when something is
missing:

- install the extension: `gh extension install github/gh-stack`
- authenticate: `gh auth login`

No version pinning, no auto-install, no other runtime checks. See the README
for full setup.

Binary name: `jj-spr`. Suggested module path: `github.com/levibostian/jj-spr`.

## 5. Domain model

| Concept | Representation |
|---|---|
| Trunk / base branch | jj revset `trunk()`, assumed to be the GitHub default branch. Resolve the branch name with `gh repo view --json defaultBranchRef`, or `--base`. |
| Stack label | The single non-tool local jj bookmark the user maintains. |
| Stack commits | `trunk()..H`, oldest → newest, where `H` is the change ID the bookmark points to. |
| Commit identity | Full jj **change ID** (stable across rewrites/reorders). |
| Per-commit remote branch | `<stack-bookmark>-<change-id>` → e.g. bookmark `mystack`, change `ymnkp...` → `mystack-ymnkqpulnsvvxvtnvwqnwnpwtqukztxt`. |
| Per-commit local bookmark | Same name as the remote branch. Persisted so jj can force-update the remote branch and so the PR association is rediscovered by change ID. |
| PR association | Look up PRs by head ref (`pulls?head=OWNER:<branch>&state=all`). No local mapping file. |
| PR title | First line of the jj commit description. |
| PR body | Remainder of the jj commit description after the first line. |
| GitHub stack | Created/updated exclusively by `gh stack link <pr...>` (bottom → top). Reconciled via the Stacks REST API when membership must change. |

Assumption: `trunk()` and the GitHub default branch are the same branch. The
tool never tries to reconcile a mismatch; `--base` overrides the branch name
if needed.

### Why change-ID branch names

The user freely rewrites, reorders, squashes, and splits commits. jj change IDs
survive all of those. Naming the remote branch after the change ID means the
same GitHub PR is updated after any rewrite; no state file is needed. Verified:
jj bookmarks follow rewritten commits automatically, so `<stack>-<changeid>`
keeps pointing at the current version.

#### Example: a rewrite keeps the same PR

Start with three commits on `mystack`. Each change ID is fixed at creation and
is *not* a commit hash, so it survives content rewrites and rebases.

```
main
 └─ A  change=xq...  "Add model"   branch mystack-xq...   PR #41
     └─ B  change=rt...  "Add API"   branch mystack-rt...   PR #42
         └─ C  change=kp...  "Add UI"  branch mystack-kp...   PR #43
mystack -> C
```

The user edits the body of B (amend/`jj describe`/`jj squash`). jj rewrites the
commit to a **new commit hash** but keeps `change=rt...`. `jj-spr update`
recomputes `branch = mystack-rt...`, looks it up with
`pulls?head=OWNER:mystack-rt...`, and finds PR #42 again. It force-updates the
branch to B's new content and runs `gh pr edit #42`. PR #42 keeps its reviews
and comments; no new PR is opened.

If branches were named by stack position (`mystack-1`, `mystack-2`, …), a rewrite could
shift positions and silently attach the wrong commit to a PR. Change IDs make
the association content-independent.

#### Example: a reorder keeps each PR with its commit

The user reorders so C is below B:

```
main
 └─ A  xq...  PR #41
     └─ C  kp...  PR #43
         └─ B  rt...  PR #42
mystack -> B
```

`jj-spr update` still maps `mystack-rt...`→PR #42 and `mystack-kp...`→PR #43, then
re-stacks them in the new order (unstack + `gh stack link`). The PRs move
position in the stack; their numbers and review history do not change.

#### Example: post-merge result

`main` lands PR #41 (bottom of the stack) in the GitHub UI. GitHub
automatically retargets #42 onto `main` so it stays reviewable. The user then
runs `jj git fetch` and `jj-spr sync`:

- #41 is classified retired (`merged_at != null`) and is no longer published.
- A is already in `trunk()` (rebase merge) or its tree content is in `trunk()`
  (squash merge); `jj rebase -s <first-active> -o 'trunk()'` drops A from the
  active history.
- #42 and #43 keep their numbers (same change IDs) and are re-pushed on top of
  the updated `trunk()`, with bases re-chained.
- The native stack on GitHub now contains only the remaining open PRs; the
  GitHub diffs show only each PR's own changes.

## 6. CLI surface

```
jj-spr update [flags]   # create/update PRs and the GitHub stack
jj-spr status [flags]   # show the stack and per-PR status
jj-spr sync   [flags]   # jj git fetch, rebase the stack on trunk, then update
```

Persistent flags:

| Flag | Default | Meaning |
|---|---|---|
| `--stack <name>` | auto-detect | User bookmark naming the stack. |
| `--base <branch>` | repo default branch | Base branch of the bottom PR. |
| `--remote <name>` | `origin` | Remote for `jj git fetch` / `jj git push`. |
| `--draft` | off | Create new PRs as drafts (existing draft state is not touched). |
| `--json` | off | `status` only: machine-readable output. |
| `-v/--verbose` | off | Log the exact `jj`/`gh` commands run. |

No other commands. `--help` / `--version` are the only extras.

### Stack auto-detection

1. List local bookmarks:
   `jj bookmark list -T 'name ++ "\t" ++ if(normal_target, normal_target.change_id(), "") ++ "\n"'`
   (bookmarks with no normal target are ignored).
2. Drop per-commit tool bookmarks. A local bookmark `B` with target change ID
   `C` is a tool bookmark when `B` ends with `"-" + C` (the tool names each
   per-commit branch `<stack>-<change-id>`); every other local bookmark is a
   candidate.
3. Keep only candidates whose target change ID `C` satisfies
   `jj log -r "<C> & ~::trunk()" --no-graph -T 'change_id'` → non-empty
   (not an ancestor of trunk; a stack bookmark may sit behind trunk right
   after a fetch, so "descendant of trunk" is deliberately not required).
4. If `--stack` is given, use it. Else if exactly one candidate, use it. Else
   error and list candidates, telling the user to pass `--stack`.

## 7. Core algorithms

### 7.1 Read the stack commits

Let `H` be the change ID the chosen stack bookmark points to (from §6). Using
the resolved change ID instead of the raw bookmark name avoids revset quoting
issues with bookmark names.

```
jj log --no-graph --reversed \
  -r 'trunk()..H' \
  -T 'json(self) ++ "\n"'
```

`json(self)` yields `commit_id`, `change_id`, `description`, `parents`.
`--reversed` gives bottom → top. Parse each line with `encoding/json`.

For each commit compute:

- `branch = <stack-bookmark> + "-" + change_id`.
- `empty` via
  `jj log -r '<change_id>' --no-graph -T 'if(empty, "1", "0")'`.
- Title/body from `description`: title = first line trimmed; body = text after
  the first `\n`, trimmed of one leading blank line.

Skip **empty** commits (they have no diff and GitHub rejects empty PRs). Log
them as skipped.

### 7.2 Resolve each commit's PR

Look up by head ref (works for merged PRs even after the branch is deleted):

```
gh api -X GET repos/{owner}/{repo}/pulls \
  -f head='<owner>:<branch>' -f state=all -f per_page=100 \
  --jq '.[0] // empty'
```

`gh` substitutes `{owner}`/`{repo}` from the current repo. Owner can be read
from `gh repo view --json owner --jq .owner.login` once per run.

Classify:

| Result | Meaning | Action |
|---|---|---|
| no PR | new | create |
| `state == open` | active | update title/body/base |
| `merged_at != null` | merged | **retired** (skip) |
| `state == closed` and not merged | closed | **retired** (skip, warn) |

Active PRs keep their stack order. Retired commits are removed from the
published list; the remaining active PRs are re-chained.

### 7.3 Journal: bookmarks + push

For every **active** commit:

```
jj bookmark set --allow-backwards -r '<change_id>' '<branch>'
```

Then push all active branches in one command:

```
jj git push --remote <remote> --allow-empty-description -b '<branch1>' -b '<branch2>' ...
```

`--allow-empty-description` is required: a stack branch's history can include
an empty, undescribed working-copy commit, and jj refuses to push those by
default.

If the push fails (e.g. stale remote-tracking after an external change), stop
before touching PRs and tell the user to run `jj git fetch`. jj's built-in
safety checks already behave like `--force-with-lease`, so rewritten branches
update correctly after a fetch.

### 7.4 Create / update PRs

Create a new PR (branch already pushed; `--head` prevents any push):

```
gh pr create \
  --head <branch> --base <base> \
  --title <title> --body-file - \
  [--draft]
```

Base for the bottom active PR is the trunk branch; for every other active PR
it is the previous active PR's branch name. Feed the body on stdin. Parse the
PR number from the printed URL (`.../pull/<n>`).

Update an existing active PR only when something changed to avoid notification
noise. Compare against the PR fields from §7.2 first:

```
gh pr edit <number> --title <title> --body-file -
gh pr edit <number> --base <base>     # only if base differs
```

Base is normally corrected by `gh stack link`; setting it explicitly is needed
only for a lone active PR (§7.6).

### 7.5 Reconcile the GitHub stack

`gh stack link` is the primary mechanism, but it is **append-only**: it can
create a stack and append PRs on top, and it refuses to remove any PR already
in the stack. It also refuses to touch a stack that still lists merged PRs.
We therefore read the current stack first and reset it when the desired
composition is not a pure append.

Desired list `D` = active PR numbers, bottom → top.

```
if len(D) < 2:
    if any PR in D belongs to a stack:      # look it up as below, then:
        gh api -X POST repos/{owner}/{repo}/stacks/<n>/unstack
    if len(D) == 1:
        gh pr edit <D[0]> --base <base>
    return

current = first entry of
    gh api "repos/{owner}/{repo}/stacks?pull_request=<D[0]>"

if current == nil:
    gh stack link --base <base> <D...>
    return

active = [pr.number for pr in current.pull_requests
            if pr.state == "open" and pr.merged_at == null]
hasMerged = any pr.merged_at != null in current.pull_requests

if !hasMerged && isOrderedPrefix(active, D):
    gh stack link --base <base> <D...>       # create/append/fix bases
else:
    gh api -X POST repos/{owner}/{repo}/stacks/<current.number>/unstack
    gh stack link --base <base> <D...>       # recreate in the new order
```

Notes:

- `isOrderedPrefix(a, d)` means every element of `a` equals `d[i]` for
  `i < len(a)`.
- `unstack` removes every **eligible** (open, non-queued) PR from the stack;
  merged PRs are left behind in the old stack. The new stack is created from
  the open PRs only, which is exactly what we want after a partial merge.
- Passing `--base` is mandatory: it prevents `gh stack link` from calling
  `git.DefaultBranch()` and keeps behavior explicit.
- `gh stack link` re-fixes each existing PR's base to the correct chain
  *before* it creates/updates the stack, so base chaining is correct even on
  reorder.
- If `gh stack link` reports a 422 ("already part of a different stack"), the
  desired PRs are split across stacks. Print the error and suggest unstacking
  the PRs on GitHub. Do not attempt to repair automatically.

### 7.6 `update`

1. Resolve the bookmark (§6).
2. Read commits (§7.1), classify PRs (§7.2).
3. Abort if any commit in the range has a conflict:
   `jj log -r 'conflicts() & (trunk()..H)' --no-graph -T 'change_id'`
   non-empty → error "resolve conflicts and run `jj-spr update` again".
4. If there are no active commits: report, reconcile to an empty/dissolved
   stack (§7.5 with `D` empty), and exit 0.
5. Push active branches (§7.3).
6. Create/update active PRs (§7.4).
7. Reconcile the stack (§7.5).
8. Print a short result: retired PRs, updated PRs, created PRs, stack number.

`update` never fetches and never rebases. The user owns history movement.

### 7.7 `sync`

`sync` is `update` plus the post-merge reconciliation the user would otherwise
do by hand. It is the only command that rewrites history.

1. `jj git fetch --remote <remote>`.
2. Resolve the bookmark and read commits; classify PRs.
3. Determine the first **active** commit (oldest unretired, non-empty).
4. If there is at least one active commit:
   ```
   jj rebase -s '<first-active-change-id>' -o 'trunk()'
   ```
   This drops retired/merged ancestors from the local history (squash merges)
   and moves the active stack onto the updated trunk (rebase merges). jj
   no-ops if the parent already is the trunk tip.
5. Conflict check: `jj log -r 'conflicts() & ::H'` non-empty → stop with a
   clear message. Do not push.
6. Run steps 5–8 of `update` (push, create/update PRs, reconcile, report).

Bookmarks follow the rewritten commits automatically, so `<stack>-<changeid>`
keeps mapping to the same PR.

### 7.8 `status`

Display-only; never mutates.

1. Resolve the bookmark and read commits/PRs as in `update`.
2. For each commit bottom → top, show: four status bits, PR number/title,
   head branch, state, URL.
3. Status bits (reuse spr's legend):
   - **checks**: from `statusCheckRollup` (pass / fail / pending / none).
   - **review**: from `reviewDecision` (approved / changes-requested /
     required / none).
   - **merge**: from `mergeable` / `mergeStateStatus` (clean / conflicts /
     blocked / draft).
   - **stack**: all PRs below are green.
4. Fetch PR detail with one `gh pr view <branch> --json
   number,title,state,isDraft,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup,url,baseRefName`
   per active PR (small stacks), or one `gh pr list --state all --json ...`
   filtered by `headRefName` for efficiency.
5. `--json` prints the same model as JSON for scripting.

## 8. Exact command reference

| Purpose | Command |
|---|---|
| List local bookmarks | `jj bookmark list -T 'name ++ "\t" ++ if(normal_target, normal_target.change_id(), "") ++ "\n"'` |
| Check bookmark is not below trunk | `jj log -r '<C> & ~::trunk()' --no-graph -T 'change_id'` |
| Read commits | `jj log --no-graph --reversed -r 'trunk()..H' -T 'json(self) ++ "\n"'` (`H` = bookmark target change ID) |
| Check empty / conflict | `jj log -r '<rev>' --no-graph -T 'if(empty, "1", "0")'` / `'if(conflict, "1", "0")'` |
| Set per-commit bookmark | `jj bookmark set --allow-backwards -r '<change_id>' '<branch>'` |
| Push branches | `jj git push --remote <remote> --allow-empty-description -b '<b1>' -b '<b2>'` |
| Fetch | `jj git fetch --remote <remote>` |
| Rebase stack onto trunk | `jj rebase -s '<first-active-change-id>' -o 'trunk()'` |
| Default branch | `gh repo view --json defaultBranchRef --jq '.defaultBranchRef.name'` |
| Repo owner | `gh repo view --json owner --jq '.owner.login'` |
| Find PR by head | `gh api -X GET repos/{owner}/{repo}/pulls -f head='<owner>:<branch>' -f state=all --jq '.[0] // empty'` |
| Create PR | `gh pr create --head <branch> --base <base> --title <t> --body-file - [--draft]` |
| Update PR | `gh pr edit <number> --title <t> --body-file -` / `--base <base>` |
| PR detail (status) | `gh pr view <branch> --json number,title,state,isDraft,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup,url,baseRefName` |
| Find stack for PR | `gh api "repos/{owner}/{repo}/stacks?pull_request=<n>"` |
| Unstack | `gh api -X POST repos/{owner}/{repo}/stacks/<n>/unstack` |
| Link/create/append stack | `gh stack link --base <base> <pr1> <pr2> ...` |

## 9. Error handling & edge cases

- **Not a jj repo / no `trunk()`**: fail early with an actionable message
  (requires an `origin` remote and a fetched default branch).
- **Bookmark ambiguity**: list candidates, require `--stack`.
- **No active commits**: exit 0 with "nothing to publish"; drop any existing
  stack membership.
- **Conflicted commits**: never push. Abort with the conflicting change IDs.
- **Empty commits**: skipped (reported).
- **Closed (not merged) PR**: treated as retired; warn so the user can reopen
  or drop the commit.
- **Merged PRs still in the remote stack**: force the unstack + recreate path
  (do not rely on `gh stack link`'s append path).
- **Reorder / middle insert / middle removal**: force unstack + recreate.
- **`gh stack link` 422 "different stack"**: print and stop.
- **Branch push rejected by jj safety check**: stop, tell the user to
  `jj git fetch` and retry.
- **A single active PR**: no GitHub stack exists (min size 2); ensure its base
  is trunk and make sure it is not left inside an old stack.
- **No local changes in title/body**: skip `gh pr edit` to avoid notifications.
- **`--draft`**: applies only to newly created PRs. Existing draft state is
  left to `gh pr ready`.

## 10. Go project layout

```
go.mod                      # module github.com/levibostian/jj-spr, Go 1.26
main.go                     # subcommand dispatch (stdlib flag; no CLI deps)
internal/jj/jj.go           # exec jj: run, log JSON, bookmark set, push,
                            #   fetch, rebase, revset predicates
internal/gh/gh.go           # exec gh: api, pr create/edit/view, stack link,
                            #   repo view
internal/runner/runner.go   # Runner interface { Run(ctx, name, args, stdin) }
                            #   + exec implementation + fake for tests
internal/stack/stack.go     # domain types + build: Bookmark, Commit, PR, Stack
internal/stack/resolve.go   # bookmark auto-detection, commit enumeration,
                            #   retirement classification, branch naming
internal/stack/publish.go   # push, PR create/update, reconcile (§7.5)
internal/stack/sync.go      # fetch, rebase, conflict gate
internal/stack/status.go    # status bits + rendering
cmd/                        # update.go, status.go, sync.go (thin wiring)
```

Design rules:

- All external calls go through `runner.Runner` so tests inject fakes.
- One file per concern; the domain package must not know about `flag`.
- No third-party dependencies required. `gh`/`jj` do the heavy lifting.

## 11. Testing

- **Unit tests** (`internal/stack`): commit enumeration and retirement
  classification; `isOrderedPrefix`; reconcile decision table
  (nil / prefix / reorder / has-merged / single PR); status-bit computation.
  Use fake runners returning canned `jj`/`gh` output.
- **Golden tests** for `status` rendering.
- **Integration smoke test** (manual, documented in README):
  1. Create a colocated jj repo with a GitHub origin and 3 commits.
  2. `jj bookmark create mystack -r @-` (or at `@`).
  3. `jj-spr update` → 3 stacked PRs.
  4. Amend a middle commit, `jj-spr update` → same PRs updated, order intact.
  5. Reorder commits, `jj-spr update` → stack recreated in new order.
  6. Merge the bottom PR in the UI, `jj git fetch`, `jj-spr sync` → remaining
     PRs re-based and re-stacked.
  7. `jj-spr status` at each step.
- CI: `go vet ./...` and `go test ./...`.

## 12. Milestones

1. `runner` + `internal/jj` wrappers + bookmark/commit resolution + tests.
2. `internal/gh` wrappers + PR lookup/create/update.
3. `update`: push → PRs → reconcile → report. Manual smoke test.
4. `status` (table + `--json`).
5. `sync`: fetch → rebase → conflict gate → publish.
6. Edge-case hardening (retired PRs, single PR, reorder, resumed stacks).
7. README + DEVELOPMENT notes.

## 13. Open questions / decisions to keep explicit

- **Supported environment is a colocated repo.** `gh stack link` has no
  `--repo`, so we cannot target a repo from a non-colocated jj workdir.
- **Recreate changes the stack number.** Reordering or removing a PR
  mid-stack cannot be expressed by `gh stack link`; we unstack and recreate.
  The old (possibly merged-only) stack object remains on GitHub as history.
- **No local state file.** PR identity comes from change-ID branch names plus
  `pulls?head=` lookups. This trades a small amount of per-run API traffic for
  zero stale state.
- **`gh stack link` needs a valid base→head chain at create time.** We set
  bases on create and let `link` re-fix them on every run.
- **Default draft off**, matching spr. `--draft` opts in for new PRs only.
