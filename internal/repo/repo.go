package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
)

// Repository wraps a go-git repo with helpers Sentinel needs.
type Repository struct {
	Path string
	raw  *git.Repository
}

// Open resolves path to an absolute path and opens it with go-git.
// Returns a clear error if it's not a git repo.
func Open(path string) (*Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("path does not exist: %s", abs)
	}

	raw, err := git.PlainOpenWithOptions(abs, &git.PlainOpenOptions{
		DetectDotGit: true, // walk up to find .git like real git does
	})
	if err != nil {
		return nil, fmt.Errorf("not a git repository (or any parent up to root): %s", abs)
	}

	return &Repository{Path: abs, raw: raw}, nil
}

// HeadInfo returns the current branch name and short commit hash.
func (r *Repository) HeadInfo() (branch, shortHash string, err error) {
	ref, err := r.raw.Head()
	if err != nil {
		return "", "", fmt.Errorf("reading HEAD: %w", err)
	}

	hash := ref.Hash().String()
	if len(hash) > 7 {
		hash = hash[:7]
	}

	name := ref.Name().Short() // "main", "feature/x", etc.
	return name, hash, nil
}

// CommitCount returns total number of commits reachable from HEAD.
// Capped at max to stay fast on huge repos.
func (r *Repository) CommitCount(max int) (int, error) {
	ref, err := r.raw.Head()
	if err != nil {
		return 0, fmt.Errorf("reading HEAD: %w", err)
	}

	iter, err := r.raw.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return 0, fmt.Errorf("walking commits: %w", err)
	}
	defer iter.Close()

	count := 0
	for {
		_, err := iter.Next()
		if err != nil {
			break // io.EOF or done
		}
		count++
		if max > 0 && count >= max {
			break
		}
	}
	return count, nil
}

// RemoteURL returns the fetch URL of the "origin" remote, or empty string.
func (r *Repository) RemoteURL() string {
	remote, err := r.raw.Remote("origin")
	if err != nil {
		return ""
	}
	urls := remote.Config().URLs
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}
