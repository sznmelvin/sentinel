package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/sznmelvin/sentinel/config"
)

var markers []string

var triageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Scan local codebase for action items (TODOs, FIXMEs)",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadEnv()
		
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
		}
		
		markers = []string{"TODO", "FIXME", "BUG", "HACK"}
		if cfg != nil && len(cfg.Markers) > 0 {
			markers = cfg.Markers
		}

		repoPathToUse := repoPath
		if repoPathToUse == "" && cfg != nil {
			repoPathToUse = cfg.RepoPath
		}
		
		// 1. Open Repository
		r, err := git.PlainOpen(repoPathToUse)
		if err != nil {
			fmt.Printf("Error opening repo at %s: %v\n", repoPath, err)
			return
		}

		// 2. Get HEAD reference
		ref, err := r.Head()
		if err != nil {
			fmt.Println("Error: Could not find HEAD. Is this an empty repo?")
			return
		}

		// 3. Get the Commit object
		commit, err := r.CommitObject(ref.Hash())
		if err != nil {
			fmt.Printf("Error reading commit: %v\n", err)
			return
		}

		fmt.Printf("=== Sentinel Local Triage ===\n")
		fmt.Printf("Ref: %s\n", ref.Hash().String()[:7])
		fmt.Printf("Scanning files for markers: %v...\n\n", markers)

		// 4. Get the File Tree from the commit
		tree, err := commit.Tree()
		if err != nil {
			fmt.Printf("Error getting tree: %v\n", err)
			return
		}

		count := 0
		
		// 5. Walk the tree (Pure Go, no 'grep' command execution)
		err = tree.Files().ForEach(func(f *object.File) error {
			// Skip likely binary files or large assets to save RAM/CPU
			if isBinaryOrIgnored(f.Name) {
				return nil
			}

			// Open the file blob
			reader, err := f.Reader()
			if err != nil {
				return nil // skip unreadable
			}
			defer reader.Close()

			// Scan line by line
			scanner := bufio.NewScanner(reader)
			lineNum := 1
			for scanner.Scan() {
				line := scanner.Text()
				
				// Check for markers
				for _, marker := range markers {
					if strings.Contains(line, marker) {
						// Clean up the output (trim whitespace)
						cleanLine := strings.TrimSpace(line)
						// Truncate overly long lines
						if len(cleanLine) > 70 {
							cleanLine = cleanLine[:70] + "..."
						}
						
						fmt.Printf("[%s] %s:%d\n    👉 %s\n", marker, f.Name, lineNum, cleanLine)
						count++
					}
				}
				lineNum++
			}
			return nil
		})

		if err != nil {
			fmt.Printf("Error walking tree: %v\n", err)
		}

		fmt.Printf("\nFound %d local action items.\n", count)
	},
}

// Simple heuristic to skip binaries/vendor/git files
func isBinaryOrIgnored(path string) bool {
	lower := strings.ToLower(path)
	// Skip images, binaries, lock files, and the .git directory (though Tree walker usually skips .git)
	ignoredExts := []string{".png", ".jpg", ".exe", ".bin", ".lock", "go.sum", ".pdf", ".zip"}
	
	for _, ext := range ignoredExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	// Skip vendor directory if present
	if strings.Contains(lower, "vendor/") {
		return true
	}
	return false
}

func init() {
	rootCmd.AddCommand(triageCmd)
}