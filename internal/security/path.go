package security

import (
	"path/filepath"
	"strings"
)

// IsSafePath validates that userPath is within baseDir to prevent path traversal attacks
func IsSafePath(baseDir, userPath string) (bool, error) {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return false, err
	}

	absUserPath, err := filepath.Abs(userPath)
	if err != nil {
		return false, err
	}

	return strings.HasPrefix(absUserPath, absBaseDir), nil
}
