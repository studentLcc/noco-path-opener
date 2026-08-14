package gui

import (
	"os"
	"path/filepath"
	"strings"
)

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		key := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, path)
	}
	return result
}

func mostRecentSelectionDir(paths []string) string {
	for i := len(paths) - 1; i >= 0; i-- {
		path := strings.TrimSpace(paths[i])
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return path
			}
			return filepath.Dir(path)
		}
	}
	return ""
}

func selectionInitialDir(recentDir string, selectedPaths []string) string {
	if recentDir = strings.TrimSpace(recentDir); recentDir != "" {
		return recentDir
	}
	if dir := mostRecentSelectionDir(selectedPaths); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return home
}
