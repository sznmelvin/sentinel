package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sznmelvin/sentinel/config"
	"github.com/sznmelvin/sentinel/tui" 
)

var (
	repoPath string
	configPath string
)

var rootCmd = &cobra.Command{
	Use:   "sentinel",
	Short: "Sentinel: TUI for Open Source Maintainers",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadEnv()
		
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
		}
		
		if repoPath == "" && cfg != nil {
			repoPath = cfg.RepoPath
		}
		
		p := tea.NewProgram(tui.InitialModel(repoPath, cfg), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v", err)
			os.Exit(1)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&repoPath, "repo", "r", ".", "Path to local git repo")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to config file")
}