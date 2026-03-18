package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Markers    []string `yaml:"markers"`
	RepoPath   string   `yaml:"repo_path"`
	GithubToken string   `yaml:"-"`
}

var DefaultConfig = Config{
	Markers:  []string{"TODO", "FIXME", "BUG", "HACK"},
	RepoPath: ".",
}

func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig

	paths := []string{}
	if configPath != "" {
		paths = append(paths, configPath)
	}
	paths = append(paths, ".sentinel.yaml")
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".sentinel.yaml"))
	}

	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			yaml.Unmarshal(data, &cfg)
			break
		}
	}

	return &cfg, nil
}

func LoadEnv() {
	godotenv.Load()
}
