package models

import "time"

// Issue represents a normalized issue or PR, whether from
// a local git-notes cache or the GitHub API.
type Issue struct {
	Number    int
	Title     string
	Body      string
	State     string   // "open" | "closed"
	Labels    []string
	IsPR      bool
	Author    string
	CreatedAt time.Time
	UpdatedAt time.Time
	URL       string
}
