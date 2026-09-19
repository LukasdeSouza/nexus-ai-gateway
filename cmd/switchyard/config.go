package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LocalRepoConfig represents per-repository configuration saved in .switchyard.json
type LocalRepoConfig struct {
	Preset  string  `json:"preset,omitempty"`
	Policy  string  `json:"policy,omitempty"`
	Budget  float64 `json:"budget,omitempty"`
	Caveman *bool   `json:"caveman,omitempty"`
}

// loadLocalRepoConfig searches upward from current directory to find .switchyard.json
func loadLocalRepoConfig() *LocalRepoConfig {
	dir, err := os.Getwd()
	if err != nil {
		return nil
	}

	for {
		configPath := filepath.Join(dir, ".switchyard.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var cfg LocalRepoConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				return &cfg
			}
		}

		// Stop searching if we hit .git directory or root
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil
}
