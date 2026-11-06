package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Repo.GitHubHost != "github.com" || cfg.Repo.GitHubRemote != "origin" || cfg.Repo.GitHubBranch != "main" {
		t.Fatalf("repo defaults: %+v", cfg.Repo)
	}
	if !cfg.Repo.RequireChecks || !cfg.Repo.RequireApproval || cfg.Repo.MergeMethod != "rebase" || cfg.Repo.PRTemplateType != "stack" {
		t.Fatalf("repo defaults: %+v", cfg.Repo)
	}
	if !cfg.User.ShowPRLink || !cfg.User.LogGitCommands || !cfg.User.LogGitHubCalls || cfg.User.BranchPrefix != "spr" {
		t.Fatalf("user defaults: %+v", cfg.User)
	}
}

func TestLoadMissingFiles(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo.GitHubRemote != "origin" || cfg.User.BranchPrefix != "spr" {
		t.Fatalf("want defaults, got %+v", cfg)
	}
}

func TestLoadEmptyPaths(t *testing.T) {
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo.GitHubBranch != "main" {
		t.Fatalf("got %+v", cfg.Repo)
	}
}

func TestLoadRepoFile(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, ".spr.yml", `githubRemote: upstream
defaultReviewers:
  - teammate
  - acme/team
mergeMethod: squash
`)
	cfg, err := Load(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo.GitHubRemote != "upstream" || cfg.Repo.MergeMethod != "squash" {
		t.Fatalf("got %+v", cfg.Repo)
	}
	if len(cfg.Repo.DefaultReviewers) != 2 || cfg.Repo.DefaultReviewers[0] != "teammate" {
		t.Fatalf("reviewers: %+v", cfg.Repo.DefaultReviewers)
	}
	// A field not in the file keeps its default.
	if cfg.Repo.GitHubBranch != "main" {
		t.Fatalf("want default branch, got %q", cfg.Repo.GitHubBranch)
	}
}

func TestLoadUserFile(t *testing.T) {
	home := t.TempDir()
	writeFile(t, home, ".spr.yml", `createDraftPRs: true
branchPrefix: mystack
`)
	cfg, err := Load("", home)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.User.CreateDraftPRs || cfg.User.BranchPrefix != "mystack" {
		t.Fatalf("got %+v", cfg.User)
	}
	if !cfg.User.ShowPRLink {
		t.Fatal("want user default preserved")
	}
}

func TestLoadRepoAndUserFiles(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	writeFile(t, repo, ".spr.yml", "defaultReviewers:\n  - teammate\n")
	writeFile(t, home, ".spr.yml", "showCommitID: true\n")
	cfg, err := Load(repo, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repo.DefaultReviewers) != 1 || !cfg.User.ShowCommitID {
		t.Fatalf("got %+v", cfg)
	}
}

func TestLoadBadYAML(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, ".spr.yml", "defaultReviewers: [unterminated\n")
	if _, err := Load(repo, ""); err == nil {
		t.Fatal("want error")
	}
}

// TestLoadMatchesSprSchema pins every field name spr writes so the same
// .spr.yml keeps working.
func TestLoadMatchesSprSchema(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, ".spr.yml", `githubRepoOwner: o
githubRepoName: r
githubHost: github.com
githubRemote: origin
githubBranch: main
requireChecks: true
requiredChecks:
  - ci/test
requireApproval: true
defaultReviewers:
  - teammate
mergeMethod: rebase
mergeQueue: false
prTemplateType: stack
prTemplatePath: .github/pull_request_template.md
prTemplateInsertStart: START
prTemplateInsertEnd: END
mergeCheck: make check
forceFetchTags: false
showPrTitlesInStack: false
branchPushIndividually: false
`)
	if _, err := Load(repo, ""); err != nil {
		t.Fatal(err)
	}
}
