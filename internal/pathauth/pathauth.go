package pathauth

import (
	"path/filepath"
	"runtime"
	"strings"
)

func IsAllowed(path string, allowedRoots []string) (bool, error) {
	if len(allowedRoots) == 0 {
		return true, nil
	}

	cleanPath := clean(path)
	for _, root := range allowedRoots {
		cleanRoot := clean(root)
		if isSameOrChild(cleanPath, cleanRoot) {
			return true, nil
		}
	}
	return false, nil
}

func isSameOrChild(path string, root string) bool {
	if path == root {
		return true
	}
	for _, separator := range []string{string(filepath.Separator), `\`, `/`} {
		if strings.HasPrefix(path, strings.TrimRight(root, `\/`)+separator) {
			return true
		}
	}
	return false
}

func clean(value string) string {
	cleaned := filepath.Clean(value)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}
