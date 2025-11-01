package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/wanetty/upgopher/internal/security"
	"github.com/wanetty/upgopher/internal/utils"
)

// TestIsSafePath tests the path traversal prevention function
func TestIsSafePath(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		baseDir  string
		userPath string
		want     bool
		wantErr  bool
	}{
		{
			name:     "valid path within base directory",
			baseDir:  tempDir,
			userPath: filepath.Join(tempDir, "file.txt"),
			want:     true,
			wantErr:  false,
		},
		{
			name:     "path traversal attempt with ../",
			baseDir:  tempDir,
			userPath: filepath.Join(tempDir, "../etc/passwd"),
			want:     false,
			wantErr:  false,
		},
		{
			name:     "nested valid path",
			baseDir:  tempDir,
			userPath: filepath.Join(tempDir, "subdir/file.txt"),
			want:     true,
			wantErr:  false,
		},
		{
			name:     "exact base directory path",
			baseDir:  tempDir,
			userPath: tempDir,
			want:     true,
			wantErr:  false,
		},
		{
			name:     "absolute path outside base",
			baseDir:  tempDir,
			userPath: "/etc/passwd",
			want:     false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := security.IsSafePath(tt.baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsSafePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsSafePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSearchInFile tests the file search functionality
func TestSearchInFile(t *testing.T) {
	// Create temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	content := `Line 1: Hello World
Line 2: hello world
Line 3: HELLO WORLD
Line 4: Testing search functionality
Line 5: Case sensitive test`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name          string
		searchTerm    string
		caseSensitive bool
		wholeWord     bool
		wantMatches   int
	}{
		{
			name:          "case insensitive search",
			searchTerm:    "hello",
			caseSensitive: false,
			wholeWord:     false,
			wantMatches:   3, // matches all three "hello/Hello/HELLO"
		},
		{
			name:          "case sensitive search",
			searchTerm:    "Hello",
			caseSensitive: true,
			wholeWord:     false,
			wantMatches:   1, // matches only "Hello"
		},
		{
			name:          "whole word search",
			searchTerm:    "test",
			caseSensitive: false,
			wholeWord:     true,
			wantMatches:   1, // matches "test" but not "Testing"
		},
		{
			name:          "no matches",
			searchTerm:    "xyz123nonexistent",
			caseSensitive: false,
			wholeWord:     false,
			wantMatches:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := utils.SearchInFile(testFile, tt.searchTerm, tt.caseSensitive, tt.wholeWord)
			if err != nil {
				t.Errorf("SearchInFile() error = %v", err)
				return
			}

			// Filter out "No matches found" special result (line number -1)
			var actualMatches []utils.SearchResult
			for _, r := range results {
				if r.LineNumber != -1 {
					actualMatches = append(actualMatches, r)
				}
			}

			if len(actualMatches) != tt.wantMatches {
				t.Errorf("searchInFile() got %d matches (want %d) for term '%s'", len(actualMatches), tt.wantMatches, tt.searchTerm)
			}
		})
	}
}

// TestSearchInFileNonExistent tests searching in non-existent file
func TestSearchInFileNonExistent(t *testing.T) {
	_, err := utils.SearchInFile("/nonexistent/file.txt", "test", false, false)
	if err == nil {
		t.Error("searchInFile() expected error for non-existent file, got nil")
	}
}

// TestBase64PathEncoding tests base64 encoding/decoding of file paths
func TestBase64PathEncoding(t *testing.T) {
	tests := []struct {
		name         string
		originalPath string
	}{
		{
			name:         "simple filename",
			originalPath: "file.txt",
		},
		{
			name:         "path with subdirectory",
			originalPath: "subdir/file.txt",
		},
		{
			name:         "path with spaces",
			originalPath: "my folder/my file.txt",
		},
		{
			name:         "path with special characters",
			originalPath: "folder (1)/file [test].txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := base64.StdEncoding.EncodeToString([]byte(tt.originalPath))

			// Decode
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Errorf("Failed to decode base64: %v", err)
				return
			}

			// Verify round-trip
			if string(decoded) != tt.originalPath {
				t.Errorf("Base64 round-trip failed: got %s, want %s", string(decoded), tt.originalPath)
			}
		})
	}
}

// TestCheckRateLimit tests the rate limiting functionality
func TestCheckRateLimit(t *testing.T) {
	// Use unique IP for this test to avoid interference
	ip := "test.192.168.1.100"

	// First 20 requests should pass
	for i := 0; i < 20; i++ {
		if !security.CheckRateLimit(ip) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 21st request should be blocked
	if security.CheckRateLimit(ip) {
		t.Error("Request 21 should be blocked by rate limit")
	}
}

// TestFormatFileSize tests file size formatting
func TestFormatFileSizeLogic(t *testing.T) {
	tests := []struct {
		name      string
		bytes     int64
		wantValue float64
		wantUnit  string
	}{
		{
			name:      "bytes",
			bytes:     500,
			wantValue: 500,
			wantUnit:  "bytes",
		},
		{
			name:      "kilobytes",
			bytes:     2000,
			wantValue: 2.0,
			wantUnit:  "KBytes",
		},
		{
			name:      "megabytes",
			bytes:     5000000, // 5 MB in decimal
			wantValue: 5.0,
			wantUnit:  "MBytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, unit := utils.FormatFileSize(tt.bytes)

			// Allow small floating point differences
			if value < tt.wantValue-0.1 || value > tt.wantValue+0.1 {
				t.Errorf("FormatFileSize() value = %v, want ~%v", value, tt.wantValue)
			}

			if unit != tt.wantUnit {
				t.Errorf("FormatFileSize() unit = %v, want %v", unit, tt.wantUnit)
			}
		})
	}
}
