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

	// Require separator suffix to prevent false positives where a directory
	// name is a prefix of another (e.g. /uploads matching /uploads2/file).
	return absUserPath == absBaseDir ||
		strings.HasPrefix(absUserPath, absBaseDir+string(filepath.Separator)), nil
}
