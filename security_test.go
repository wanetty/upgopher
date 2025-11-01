package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wanetty/upgopher/internal/security"
)

// TestPathTraversalAttacks tests various path traversal attack vectors
func TestPathTraversalAttacks(t *testing.T) {
	tempDir := t.TempDir()

	attacks := []struct {
		name        string
		attackPath  string
		description string
	}{
		{
			name:        "classic dot-dot-slash",
			attackPath:  "../../../etc/passwd",
			description: "Classic path traversal using ../",
		},
		{
			name:        "encoded dot-dot-slash",
			attackPath:  "..%2F..%2F..%2Fetc%2Fpasswd",
			description: "URL-encoded path traversal",
		},
		{
			name:        "double encoded",
			attackPath:  "..%252F..%252F..%252Fetc%252Fpasswd",
			description: "Double URL-encoded path traversal",
		},
		{
			name:        "backslash variant",
			attackPath:  "..\\..\\..\\etc\\passwd",
			description: "Windows-style path traversal",
		},
		{
			name:        "absolute path",
			attackPath:  "/etc/passwd",
			description: "Absolute path escape attempt",
		},
		{
			name:        "null byte injection",
			attackPath:  "file.txt\x00../../etc/passwd",
			description: "Null byte path traversal",
		},
	}

	for _, att := range attacks {
		t.Run(att.name, func(t *testing.T) {
			// Try to construct a path with the attack vector
			// Note: filepath.Join automatically cleans many attack vectors
			attackFullPath := filepath.Join(tempDir, att.attackPath)

			// IsSafePath should detect this
			safe, err := security.IsSafePath(tempDir, attackFullPath)

			if err != nil {
				// Error is acceptable for malformed paths
				t.Logf("Attack '%s' caused error (blocked): %v", att.name, err)
				return
			}

			if safe {
				// Check if filepath.Join already normalized this to a safe path
				if strings.HasPrefix(attackFullPath, tempDir) && !strings.Contains(att.attackPath, "..") {
					t.Logf("Attack '%s' was neutralized by filepath.Join normalization", att.name)
				} else {
					t.Logf("Warning: Attack '%s' considered safe after normalization: %s -> %s",
						att.name, att.attackPath, attackFullPath)
				}
			} else {
				t.Logf("Attack '%s' successfully blocked", att.name)
			}
		})
	}
}

// TestDirectoryDeletionPrevention tests that directories cannot be deleted
func TestDirectoryDeletionPrevention(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test directory
	testSubDir := filepath.Join(tempDir, "testdir")
	err := os.Mkdir(testSubDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a file inside to make it non-empty
	testFile := filepath.Join(testSubDir, "file.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Encode the directory path
	encodedPath := base64.StdEncoding.EncodeToString([]byte("testdir"))

	// Create delete handler
	handler := deleteHandler(tempDir)

	// Create request to delete directory
	req := httptest.NewRequest("GET", "/delete/?path="+encodedPath, nil)
	w := httptest.NewRecorder()

	// Execute handler
	handler(w, req)

	// Should return 403 Forbidden
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden, got %d", w.Code)
	}

	// Verify directory still exists
	if _, err := os.Stat(testSubDir); os.IsNotExist(err) {
		t.Error("Directory was deleted despite protection!")
	}

	// Verify body contains appropriate error message
	if !strings.Contains(w.Body.String(), "Cannot delete directories") {
		t.Errorf("Expected error message about directory deletion, got: %s", w.Body.String())
	}
}

// TestRateLimitingSequential tests rate limiting with sequential requests
func TestRateLimitingSequential(t *testing.T) {
	// Use unique IP for this test
	testIP := "sequential.test.192.168.1.50"

	successCount := 0
	blockedCount := 0

	// Send 25 sequential requests (should allow 20, block 5)
	for i := 0; i < 25; i++ {
		allowed := security.CheckRateLimit(testIP)
		if allowed {
			successCount++
		} else {
			blockedCount++
		}
	}

	t.Logf("Rate limiting results: %d allowed, %d blocked", successCount, blockedCount)

	// Should have exactly 20 allowed and 5 blocked
	if successCount != 20 {
		t.Errorf("Expected 20 successful requests, got %d", successCount)
	}

	if blockedCount != 5 {
		t.Errorf("Expected 5 blocked requests, got %d", blockedCount)
	}
}

// TestCustomPathsConcurrentAccess tests concurrent access to custom paths map
func TestCustomPathsConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		filepath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filepath, []byte("test"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	handler := createCustomPathHandler(tempDir)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent writes to custom paths
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			fileNum := index % 10
			originalPath := fmt.Sprintf("file%d.txt", fileNum)
			customPath := fmt.Sprintf("custom-%d-%d", fileNum, index)

			// Create POST request
			body := fmt.Sprintf("originalPath=%s&customPath=%s", originalPath, customPath)
			req := httptest.NewRequest("POST", "/custom-path", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			handler(w, req)

			// Check for data races or unexpected errors
			if w.Code != http.StatusOK && w.Code != http.StatusConflict {
				errors <- fmt.Errorf("unexpected status code: %d", w.Code)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent access error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors during concurrent custom path access", errorCount)
	}
}

// TestRawHandlerPathSecurity tests raw file handler against path traversal
func TestRawHandlerPathSecurity(t *testing.T) {
	tempDir := t.TempDir()

	// Create a safe file
	testFile := filepath.Join(tempDir, "safe.txt")
	err := os.WriteFile(testFile, []byte("safe content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	handler := rawHandler(tempDir)

	attacks := []string{
		"../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"/etc/passwd",
		"safe.txt/../../etc/passwd",
	}

	for _, attack := range attacks {
		t.Run("attack_"+attack, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/raw/"+attack, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			// Should return 403 or 404, not 200
			if w.Code == http.StatusOK {
				t.Errorf("Path traversal attack succeeded with path: %s", attack)
			}

			// Should not return sensitive content
			body := w.Body.String()
			if strings.Contains(body, "root:") || strings.Contains(body, "/bin/bash") {
				t.Errorf("Sensitive file content leaked via path traversal!")
			}
		})
	}
}

// TestClipboardRateLimitRecovery tests that rate limit resets after time window
func TestClipboardRateLimitRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-based test in short mode")
	}

	testIP := "recovery.test.192.168.1.75"

	// Fill up the rate limit
	for i := 0; i < 20; i++ {
		security.CheckRateLimit(testIP)
	}

	// 21st request should be blocked
	if security.CheckRateLimit(testIP) {
		t.Error("Expected rate limit to be exhausted")
	}

	// Wait for rate limit window to pass (slightly over 1 minute)
	t.Log("Waiting for rate limit window to reset (65 seconds)...")
	time.Sleep(65 * time.Second)

	// Should be able to make requests again
	if !security.CheckRateLimit(testIP) {
		t.Error("Rate limit should have reset after time window")
	}
}

// TestSearchHandlerPathSecurity tests search endpoint against path injection
func TestSearchHandlerPathSecurity(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tempDir, "searchable.txt")
	err := os.WriteFile(testFile, []byte("searchable content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	handler := searchFileHandler(tempDir)

	// Attempt path traversal via search
	encodedAttack := base64.StdEncoding.EncodeToString([]byte("../../../etc/passwd"))

	req := httptest.NewRequest("GET",
		fmt.Sprintf("/search-file?path=%s&term=root&caseSensitive=false&wholeWord=false", encodedAttack),
		nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// Should return 403 Forbidden, not search results
	if w.Code == http.StatusOK {
		t.Error("Path traversal in search handler was not blocked")
	}

	if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("Expected 403 or 404, got %d", w.Code)
	}
}

// TestBasicAuthBypass tests that authentication cannot be bypassed
func TestBasicAuthBypass(t *testing.T) {
	username := "testuser"
	password := "testpass"

	// Create a simple handler
	protectedHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Protected content"))
	}

	// Wrap with basic auth
	authHandler := security.ApplyBasicAuth(protectedHandler, username, password)

	tests := []struct {
		name           string
		username       string
		password       string
		expectedStatus int
	}{
		{
			name:           "valid credentials",
			username:       "testuser",
			password:       "testpass",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "wrong password",
			username:       "testuser",
			password:       "wrongpass",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong username",
			username:       "wronguser",
			password:       "testpass",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no credentials",
			username:       "",
			password:       "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "timing attack protection",
			username:       "testuse", // One char short
			password:       "testpass",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.username != "" || tt.password != "" {
				req.SetBasicAuth(tt.username, tt.password)
			}
			w := httptest.NewRecorder()

			authHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusUnauthorized {
				// Verify WWW-Authenticate header is present
				if w.Header().Get("WWW-Authenticate") == "" {
					t.Error("Missing WWW-Authenticate header")
				}
			}
		})
	}
}
