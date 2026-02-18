package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wanetty/upgopher/internal/handlers"
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
				// A "safe" result is only acceptable if filepath.Join already normalized
				// the attack into a path still within tempDir.
				resolvedWithinBase := strings.HasPrefix(attackFullPath, tempDir+string(filepath.Separator)) || attackFullPath == tempDir
				if resolvedWithinBase {
					t.Logf("Attack '%s' was neutralized by filepath.Join normalization", att.name)
				} else {
					t.Errorf("Attack '%s' escaped base directory: IsSafePath returned safe=true for path outside baseDir: %s", att.name, attackFullPath)
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

	encodedPath := base64.StdEncoding.EncodeToString([]byte("testdir"))

	fh := handlers.NewFileHandlers(tempDir, true, false, false, &showHiddenFiles, &customPaths, &customPathsMutex)
	handler := fh.Delete()

	req := httptest.NewRequest("GET", "/delete/?path="+encodedPath, nil)
	w := httptest.NewRecorder()

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

	cph := handlers.NewCustomPathHandler(tempDir, true, &customPaths, &customPathsMutex)
	handler := cph.Handle()

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
			// Status codes: 200 OK (old behavior), 303 SeeOther (redirect after creation), 409 Conflict
			if w.Code != http.StatusOK && w.Code != http.StatusConflict && w.Code != http.StatusSeeOther {
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

	fh := handlers.NewFileHandlers(tempDir, true, false, false, &showHiddenFiles, &customPaths, &customPathsMutex)
	handler := fh.Raw()

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

	// Should be able to make requests again (?tab=default route)
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

	fh := handlers.NewFileHandlers(tempDir, true, false, false, &showHiddenFiles, &customPaths, &customPathsMutex)
	handler := fh.Search()

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

// TestUploadPathTraversal tests that uploaded filenames cannot escape the upload directory
func TestUploadPathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	fh := handlers.NewFileHandlers(tempDir, true, false, false, &showHiddenFiles, &customPaths, &customPathsMutex)
	handler := fh.List()

	maliciousName := "../../outside.txt"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", maliciousName)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("malicious content"))
	writer.Close()

	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler(w, req)

	// The file must NOT have been created at the traversal path outside tempDir
	escapedPath := filepath.Clean(filepath.Join(tempDir, maliciousName))
	if !strings.HasPrefix(escapedPath, tempDir) {
		if _, statErr := os.Stat(escapedPath); !os.IsNotExist(statErr) {
			t.Errorf("Path traversal upload succeeded: file created outside upload dir at %s", escapedPath)
		}
	}
}

// TestZipPathTraversal tests that the zip endpoint cannot traverse outside the base directory
func TestZipPathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	fh := handlers.NewFileHandlers(tempDir, true, false, false, &showHiddenFiles, &customPaths, &customPathsMutex)
	handler := fh.Zip()

	encodedMalicious := base64.StdEncoding.EncodeToString([]byte("../../"))
	req := httptest.NewRequest("GET", "/zip?path="+encodedMalicious, nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Zip path traversal should return 403 Forbidden, got %d", w.Code)
	}
}

// TestClipboardTabNameInjection tests that malicious tab names are rejected by the handler.
func TestClipboardTabNameInjection(t *testing.T) {
	ch := handlers.NewClipboardHandler(true, 20)
	handle := ch.Handle()

	attacks := []struct {
		name    string
		tabName string
	}{
		{"xss script tag", "<script>alert(1)</script>"},
		{"xss img onerror", `<img src=x onerror=alert(1)>`},
		{"path traversal", "../../../etc/passwd"},
		{"sql injection", "'; DROP TABLE tabs;--"},
		{"null byte", "tab\x00name"},
		{"long name", strings.Repeat("a", 200)},
		{"unicode bypass", "\u003cscript\u003e"},
		{"shell injection", "$(rm -rf /)"},
	}

	for _, att := range attacks {
		t.Run(att.name, func(t *testing.T) {
			// url.Values ensures the name is percent-encoded so httptest.NewRequest
			// doesn't panic; the server decodes it before regex validation.
			params := url.Values{"tab": {att.tabName}}
			req := httptest.NewRequest("POST", "/clipboard?"+params.Encode(), strings.NewReader("x"))
			req.RemoteAddr = "192.168.0.1:11111"
			w := httptest.NewRecorder()
			handle(w, req)

			if w.Code == http.StatusOK {
				t.Errorf("injection attack %q was accepted (expected 400)", att.name)
			}
		})
	}
}

// TestClipboardMaxTabsExhaustion tests exhaustion with maxTabs+1 creates from concurrent goroutines.
func TestClipboardMaxTabsExhaustion(t *testing.T) {
	const maxTabs = 5
	ch := handlers.NewClipboardHandler(true, maxTabs)
	handle := ch.Handle()

	// Create maxTabs-1 extra tabs (default already occupies 1 slot)
	for i := 0; i < maxTabs-1; i++ {
		name := fmt.Sprintf("tab%d", i)
		req := httptest.NewRequest("POST", "/clipboard?tab="+name, strings.NewReader("x"))
		req.RemoteAddr = "10.1.0.1:9999"
		w := httptest.NewRecorder()
		handle(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("creating %q: expected 201, got %d: %s", name, w.Code, w.Body.String())
		}
	}

	// Next create must hit the cap
	req := httptest.NewRequest("POST", "/clipboard?tab=overflow", strings.NewReader("x"))
	req.RemoteAddr = "10.1.0.1:9999"
	w := httptest.NewRecorder()
	handle(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 at maxTabs cap, got %d: %s", w.Code, w.Body.String())
	}
}

// TestClipboardConcurrentTabCreation races concurrent goroutines each creating a unique tab.
func TestClipboardConcurrentTabCreation(t *testing.T) {
	ch := handlers.NewClipboardHandler(true, 100)
	handle := ch.Handle()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("concurrent-%d", n)
			req := httptest.NewRequest("POST", "/clipboard?tab="+name, strings.NewReader("data"))
			req.RemoteAddr = fmt.Sprintf("10.2.0.%d:8080", n%254+1)
			w := httptest.NewRecorder()
			handle(w, req)
		}(i)
	}
	wg.Wait()

	// Verify all tabs are accessible without panic
	req := httptest.NewRequest("GET", "/clipboard/tabs", nil)
	w := httptest.NewRecorder()
	ch.ListTabs()(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ListTabs after concurrent writes: expected 200, got %d", w.Code)
	}
}

// TestClipboardTokenTimingAttack verifies that wrong-token responses take roughly
// the same time as correct-token responses (constant-time compare).
func TestClipboardTokenTimingAttack(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing-sensitive test in short mode")
	}

	const iterations = 500
	ch := handlers.NewClipboardHandler(true, 10)
	handle := ch.Handle()

	// Create a protected tab and capture the token.
	setupReq := httptest.NewRequest("POST", "/clipboard?tab=timing-test", strings.NewReader("x"))
	setupReq.RemoteAddr = "127.0.0.1:10101"
	setupReq.Header.Set("X-Tab-Token-Create", "1")
	setupW := httptest.NewRecorder()
	handle(setupW, setupReq)
	if setupW.Code != 201 {
		t.Fatalf("setup failed: %d", setupW.Code)
	}
	correctToken := setupW.Header().Get("X-Generated-Token")
	wrongToken := strings.Repeat("b", 64)

	measure := func(token string) time.Duration {
		start := time.Now()
		for i := 0; i < iterations; i++ {
			req := httptest.NewRequest("GET", "/clipboard?tab=timing-test", nil)
			req.Header.Set("X-Tab-Token", token)
			w := httptest.NewRecorder()
			handle(w, req)
		}
		return time.Since(start)
	}

	correctTime := measure(correctToken)
	wrongTime := measure(wrongToken)

	// Allow 10× variance — the constant-time compare itself is negligible vs HTTP overhead,
	// but a timing leak would show orders-of-magnitude difference.
	ratio := float64(correctTime) / float64(wrongTime)
	if ratio > 10 || ratio < 0.1 {
		t.Errorf("timing ratio correct/wrong = %.2f — potential timing leak", ratio)
	}
}

// TestClipboardTokenOversizedInput verifies that a very long token header does not panic.
func TestClipboardTokenOversizedInput(t *testing.T) {
	ch := handlers.NewClipboardHandler(true, 10)
	handle := ch.Handle()

	// Create a protected tab.
	setupReq := httptest.NewRequest("POST", "/clipboard?tab=oversize", strings.NewReader("x"))
	setupReq.RemoteAddr = "127.0.0.1:10202"
	setupReq.Header.Set("X-Tab-Token-Create", "1")
	setupW := httptest.NewRecorder()
	handle(setupW, setupReq)
	if setupW.Code != 201 {
		t.Fatalf("setup failed: %d", setupW.Code)
	}

	// Send a 1 MB token header — must return 401 cleanly, not panic.
	req := httptest.NewRequest("GET", "/clipboard?tab=oversize", nil)
	req.Header.Set("X-Tab-Token", strings.Repeat("a", 1<<20))
	w := httptest.NewRecorder()
	handle(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for oversized token, got %d", w.Code)
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
