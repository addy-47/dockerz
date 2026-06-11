package git

import (
	"fmt"
	"time"

	"github.com/addy-47/dockerz/internal/logging"
)

// ChangeType represents the type of change in git
type ChangeType int

const (
	Modified ChangeType = iota
	Added
	Deleted
	Renamed
)

// FileChange represents a single file change
type FileChange struct {
	Path       string
	ChangeType ChangeType
	OldPath    string // For renames
}

// CommitInfo represents git commit information
type CommitInfo struct {
	Hash      string
	Message   string
	Author    string
	Timestamp time.Time
}

// DiffResult represents the result of a git diff operation
type DiffResult struct {
	FilesChanged []FileChange
	CommitFrom   string
	CommitTo     string
	IsClean      bool
}

// Tracker handles git change detection
type Tracker struct {
	gitRoot               string
	allUncommittedChanges []string
	allCommitChanges      map[int][]string
	lastCommit            string
	gitCacheTTL           time.Duration
	logger                *logging.Logger
	cache                 *GitCache
}

// SetGitCacheTTL sets the TTL for git operation caches (status and diff)
func (t *Tracker) SetGitCacheTTL(ttl time.Duration) {
	t.gitCacheTTL = ttl
	if t.logger != nil {
		t.logger.Debug(logging.CATEGORY_GIT, fmt.Sprintf("Git cache TTL set to %v", ttl))
	}
}
