// Package config reads the .spr.yml configuration files. The file format and
// field names match ejoffe/spr, so the same .spr.yml works with both tools.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the parsed configuration from the repo-level .spr.yml (Repo) and
// the user-level .spr.yml (User).
type Config struct {
	Repo *RepoConfig
	User *UserConfig
}

// sprConfigFile is the config file name, shared with ejoffe/spr.
const sprConfigFile = ".spr.yml"

// RepoConfig mirrors the repository-level .spr.yml, committed next to the code.
type RepoConfig struct {
	GitHubRepoOwner string `yaml:"githubRepoOwner,omitempty"`
	GitHubRepoName  string `yaml:"githubRepoName,omitempty"`
	GitHubHost      string `yaml:"githubHost"`

	GitHubRemote string `yaml:"githubRemote"`
	GitHubBranch string `yaml:"githubBranch"`

	RequireChecks    bool     `yaml:"requireChecks"`
	RequiredChecks   []string `yaml:"requiredChecks,omitempty"`
	RequireApproval  bool     `yaml:"requireApproval"`
	DefaultReviewers []string `yaml:"defaultReviewers,omitempty"`

	MergeMethod string `yaml:"mergeMethod"`
	MergeQueue  bool   `yaml:"mergeQueue"`

	PRTemplateType        string `yaml:"prTemplateType"`
	PRTemplatePath        string `yaml:"prTemplatePath,omitempty"`
	PRTemplateInsertStart string `yaml:"prTemplateInsertStart,omitempty"`
	PRTemplateInsertEnd   string `yaml:"prTemplateInsertEnd,omitempty"`

	MergeCheck string `yaml:"mergeCheck,omitempty"`

	ForceFetchTags bool `yaml:"forceFetchTags"`

	ShowPRTitlesInStack    bool `yaml:"showPrTitlesInStack"`
	BranchPushIndividually bool `yaml:"branchPushIndividually"`
}

// UserConfig mirrors the user-level .spr.yml in the home directory.
type UserConfig struct {
	ShowPRLink       bool `yaml:"showPRLink"`
	LogGitCommands   bool `yaml:"logGitCommands"`
	LogGitHubCalls   bool `yaml:"logGitHubCalls"`
	StatusBitsHeader bool `yaml:"statusBitsHeader"`
	StatusBitsEmojis bool `yaml:"statusBitsEmojis"`

	CreateDraftPRs       bool   `yaml:"createDraftPRs"`
	PreserveTitleAndBody bool   `yaml:"preserveTitleAndBody"`
	NoRebase             bool   `yaml:"noRebase"`
	NoFetch              bool   `yaml:"noFetch"`
	DeleteMergedBranches bool   `yaml:"deleteMergedBranches"`
	ShortPRLink          bool   `yaml:"shortPRLink"`
	ShowCommitID         bool   `yaml:"showCommitID"`
	BranchPrefix         string `yaml:"branchPrefix"`
}

// Default returns the built-in defaults, matching spr's.
func Default() *Config {
	return &Config{
		Repo: &RepoConfig{
			GitHubHost:      "github.com",
			GitHubRemote:    "origin",
			GitHubBranch:    "main",
			RequireChecks:   true,
			RequireApproval: true,
			MergeMethod:     "rebase",
			PRTemplateType:  "stack",
		},
		User: &UserConfig{
			ShowPRLink:       true,
			LogGitCommands:   true,
			LogGitHubCalls:   true,
			StatusBitsHeader: true,
			StatusBitsEmojis: true,
			BranchPrefix:     "spr",
		},
	}
}

// Load reads repoRoot/.spr.yml and homeDir/.spr.yml, overlaying each on the
// defaults. An empty path or a missing file is not an error; a malformed file
// is.
func Load(repoRoot, homeDir string) (*Config, error) {
	cfg := Default()
	if repoRoot != "" {
		if err := load(filepath.Join(repoRoot, sprConfigFile), cfg.Repo); err != nil {
			return nil, err
		}
	}
	if homeDir != "" {
		if err := load(filepath.Join(homeDir, sprConfigFile), cfg.User); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// load unmarshals a YAML file into into. A missing file leaves into unchanged.
func load(path string, into any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, into); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}
