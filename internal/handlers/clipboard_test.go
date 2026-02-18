package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func newTestClipboardHandler() *ClipboardHandler {
	return NewClipboardHandler(true, 5)
}

// TestClipboardDefaultTabExists verifies "default" tab is ready on init.
func TestClipboardDefaultTabExists(t *testing.T) {
	h := newTestClipboardHandler()

	req := httptest.NewRequest(http.MethodGet, "/clipboard", nil)
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestClipboardGetNonExistentTab expects 404 for an unknown tab.
func TestClipboardGetNonExistentTab(t *testing.T) {
	h := newTestClipboardHandler()

	req := httptest.NewRequest(http.MethodGet, "/clipboard?tab=doesnotexist", nil)
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestClipboardPostAndGet verifies content round-trips correctly.
func TestClipboardPostAndGet(t *testing.T) {
	h := newTestClipboardHandler()
	handle := h.Handle()
	content := "hello, shared world"

	// POST
	req := httptest.NewRequest(http.MethodPost, "/clipboard", strings.NewReader(content))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handle(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST expected 200, got %d", w.Code)
	}

	// GET
	req2 := httptest.NewRequest(http.MethodGet, "/clipboard", nil)
	w2 := httptest.NewRecorder()
	handle(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", w2.Code)
	}
	body, _ := io.ReadAll(w2.Body)
	if string(body) != content {
		t.Errorf("content mismatch: got %q, want %q", string(body), content)
	}
}

// TestClipboardNamedTab creates a tab, writes content, reads it back.
func TestClipboardNamedTab(t *testing.T) {
	h := newTestClipboardHandler()
	handle := h.Handle()

	// POST to new tab — expects 201 Created
	req := httptest.NewRequest(http.MethodPost, "/clipboard?tab=work", strings.NewReader("work content"))
	req.RemoteAddr = "127.0.0.1:22222"
	w := httptest.NewRecorder()
	handle(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// GET from named tab
	req2 := httptest.NewRequest(http.MethodGet, "/clipboard?tab=work", nil)
	w2 := httptest.NewRecorder()
	handle(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", w2.Code)
	}
	body, _ := io.ReadAll(w2.Body)
	if string(body) != "work content" {
		t.Errorf("content mismatch: got %q", string(body))
	}

	// Ensure "default" is unchanged
	req3 := httptest.NewRequest(http.MethodGet, "/clipboard", nil)
	w3 := httptest.NewRecorder()
	handle(w3, req3)
	body3, _ := io.ReadAll(w3.Body)
	if string(body3) != "" {
		t.Errorf("default tab should be empty, got %q", string(body3))
	}
}

// TestClipboardInvalidTabName rejects names that fail the regex.
func TestClipboardInvalidTabName(t *testing.T) {
	h := newTestClipboardHandler()
	handle := h.Handle()

	badNames := []string{
		"<script>alert(1)</script>",
		"../etc/passwd",
		"tab;drop",
		"tab\x00name",
		strings.Repeat("a", 51), // >50 chars
	}

	for _, name := range badNames {
		// Use url.Values so the name is properly percent-encoded in the URL;
		// the server decodes it before regex validation.
		params := url.Values{"tab": {name}}
		req := httptest.NewRequest(http.MethodPost, "/clipboard?"+params.Encode(), strings.NewReader("x"))
		req.RemoteAddr = "127.0.0.1:33333"
		w := httptest.NewRecorder()
		handle(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("tab %q: expected 400, got %d", name, w.Code)
		}
	}
}

// TestClipboardMaxTabsEnforced ensures tabs cannot exceed maxTabs.
func TestClipboardMaxTabsEnforced(t *testing.T) {
	h := NewClipboardHandler(true, 3) // max 3 (includes "default")
	handle := h.Handle()

	// Create 2 more tabs (total = 3 = maxTabs) — expects 201 Created
	for _, name := range []string{"tab1", "tab2"} {
		req := httptest.NewRequest(http.MethodPost, "/clipboard?tab="+name, strings.NewReader("x"))
		req.RemoteAddr = "127.0.0.1:44444"
		w := httptest.NewRecorder()
		handle(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("creating %q: expected 201, got %d: %s", name, w.Code, w.Body.String())
		}
	}

	// Creating a 4th tab must be rejected
	req := httptest.NewRequest(http.MethodPost, "/clipboard?tab=tab3", strings.NewReader("x"))
	req.RemoteAddr = "127.0.0.1:44445"
	w := httptest.NewRecorder()
	handle(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when exceeding maxTabs, got %d: %s", w.Code, w.Body.String())
	}
}

// TestClipboardDeleteTab deletes a non-default tab and verifies it's gone.
func TestClipboardDeleteTab(t *testing.T) {
	h := newTestClipboardHandler()
	handle := h.Handle()

	// Create tab — expects 201 Created
	req := httptest.NewRequest(http.MethodPost, "/clipboard?tab=temp", strings.NewReader("data"))
	req.RemoteAddr = "127.0.0.1:55555"
	w := httptest.NewRecorder()
	handle(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup POST failed: %d", w.Code)
	}

	// Delete tab
	req2 := httptest.NewRequest(http.MethodDelete, "/clipboard?tab=temp", nil)
	req2.RemoteAddr = "127.0.0.1:55556"
	w2 := httptest.NewRecorder()
	handle(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("DELETE expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// GET after delete must return 404
	req3 := httptest.NewRequest(http.MethodGet, "/clipboard?tab=temp", nil)
	w3 := httptest.NewRecorder()
	handle(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Errorf("after delete: expected 404, got %d", w3.Code)
	}
}

// TestClipboardDeleteDefaultForbidden ensures the "default" tab cannot be deleted.
func TestClipboardDeleteDefaultForbidden(t *testing.T) {
	h := newTestClipboardHandler()

	req := httptest.NewRequest(http.MethodDelete, "/clipboard?tab=default", nil)
	req.RemoteAddr = "127.0.0.1:66666"
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 deleting default, got %d", w.Code)
	}
}

// TestClipboardDeleteNonExistent returns 404 for an unknown tab.
func TestClipboardDeleteNonExistent(t *testing.T) {
	h := newTestClipboardHandler()

	req := httptest.NewRequest(http.MethodDelete, "/clipboard?tab=ghost", nil)
	req.RemoteAddr = "127.0.0.1:77777"
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 deleting ghost tab, got %d", w.Code)
	}
}

// TestClipboardListTabs verifies /clipboard/tabs returns JSON including "default".
func TestClipboardListTabs(t *testing.T) {
	h := newTestClipboardHandler()

	// Add a named tab first
	postReq := httptest.NewRequest(http.MethodPost, "/clipboard?tab=alpha", strings.NewReader("a"))
	postReq.RemoteAddr = "127.0.0.1:88888"
	w0 := httptest.NewRecorder()
	h.Handle()(w0, postReq)

	req := httptest.NewRequest(http.MethodGet, "/clipboard/tabs", nil)
	w := httptest.NewRecorder()
	h.ListTabs()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"default"`) {
		t.Errorf("response should contain default tab, got: %s", body)
	}
	if !strings.Contains(body, `"alpha"`) {
		t.Errorf("response should contain alpha tab, got: %s", body)
	}
}

// ── Token protection tests ────────────────────────────────────────────────────

// helper: create a protected tab, return the plaintext token.
func createProtectedTab(t *testing.T, h *ClipboardHandler, tabName string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/clipboard?tab="+tabName, strings.NewReader("initial"))
	req.RemoteAddr = "127.0.0.1:11111"
	req.Header.Set("X-Tab-Token-Create", "1")
	w := httptest.NewRecorder()
	h.Handle()(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("createProtectedTab: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	token := w.Header().Get("X-Generated-Token")
	if token == "" {
		t.Fatal("createProtectedTab: expected X-Generated-Token header, got none")
	}
	if len(token) != 64 {
		t.Fatalf("createProtectedTab: token should be 64 hex chars, got %d", len(token))
	}
	return token
}

// TestClipboardTokenCreate verifies that creating a tab with X-Tab-Token-Create:1
// returns 201 + a 64-char hex token, and lists the tab as protected.
func TestClipboardTokenCreate(t *testing.T) {
	h := newTestClipboardHandler()
	token := createProtectedTab(t, h, "secret")

	// Verify the tab is listed as protected
	req := httptest.NewRequest(http.MethodGet, "/clipboard/tabs", nil)
	w := httptest.NewRecorder()
	h.ListTabs()(w, req)
	body := w.Body.String()
	if !strings.Contains(body, `"protected":true`) {
		t.Errorf("expected protected:true in tab list, got: %s", body)
	}
	_ = token
}

// TestClipboardTokenGetWithoutToken expects 401 when accessing a protected tab with no token.
func TestClipboardTokenGetWithoutToken(t *testing.T) {
	h := newTestClipboardHandler()
	createProtectedTab(t, h, "locked")

	req := httptest.NewRequest(http.MethodGet, "/clipboard?tab=locked", nil)
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header in 401 response")
	}
}

// TestClipboardTokenGetWithCorrectToken returns 200 + content when the token is correct.
func TestClipboardTokenGetWithCorrectToken(t *testing.T) {
	h := newTestClipboardHandler()
	token := createProtectedTab(t, h, "vault")

	req := httptest.NewRequest(http.MethodGet, "/clipboard?tab=vault", nil)
	req.Header.Set("X-Tab-Token", token)
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with correct token, got %d: %s", w.Code, w.Body.String())
	}
	body, _ := io.ReadAll(w.Body)
	if string(body) != "initial" {
		t.Errorf("unexpected content: %q", string(body))
	}
}

// TestClipboardTokenGetWrongToken expects 401 when the wrong token is provided.
func TestClipboardTokenGetWrongToken(t *testing.T) {
	h := newTestClipboardHandler()
	createProtectedTab(t, h, "fort")

	req := httptest.NewRequest(http.MethodGet, "/clipboard?tab=fort", nil)
	req.Header.Set("X-Tab-Token", strings.Repeat("a", 64))
	w := httptest.NewRecorder()
	h.Handle()(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong token, got %d", w.Code)
	}
}

// TestClipboardTokenPostUpdate verifies that updating a protected tab requires the token.
func TestClipboardTokenPostUpdate(t *testing.T) {
	h := newTestClipboardHandler()
	token := createProtectedTab(t, h, "edit-test")

	// Update without token → 401
	req := httptest.NewRequest(http.MethodPost, "/clipboard?tab=edit-test", strings.NewReader("new content"))
	req.RemoteAddr = "127.0.0.1:22222"
	w := httptest.NewRecorder()
	h.Handle()(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("POST without token: expected 401, got %d", w.Code)
	}

	// Update with correct token → 200
	req2 := httptest.NewRequest(http.MethodPost, "/clipboard?tab=edit-test", strings.NewReader("new content"))
	req2.RemoteAddr = "127.0.0.1:22222"
	req2.Header.Set("X-Tab-Token", token)
	w2 := httptest.NewRecorder()
	h.Handle()(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("POST with token: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestClipboardTokenDelete verifies that deleting a protected tab requires the token.
func TestClipboardTokenDelete(t *testing.T) {
	h := newTestClipboardHandler()
	token := createProtectedTab(t, h, "del-test")

	// Delete without token → 401
	req := httptest.NewRequest(http.MethodDelete, "/clipboard?tab=del-test", nil)
	req.RemoteAddr = "127.0.0.1:33333"
	w := httptest.NewRecorder()
	h.Handle()(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("DELETE without token: expected 401, got %d", w.Code)
	}

	// Delete with correct token → 200
	req2 := httptest.NewRequest(http.MethodDelete, "/clipboard?tab=del-test", nil)
	req2.RemoteAddr = "127.0.0.1:33333"
	req2.Header.Set("X-Tab-Token", token)
	w2 := httptest.NewRecorder()
	h.Handle()(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("DELETE with token: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestClipboardDefaultTabNeverProtected verifies the default tab always allows access
// even when X-Tab-Token-Create:1 is sent (the flag is only honored for new tabs).
func TestClipboardDefaultTabNeverProtected(t *testing.T) {
	h := newTestClipboardHandler()

	// POST to default with token-create flag — must not protect the pre-existing tab
	req := httptest.NewRequest(http.MethodPost, "/clipboard", strings.NewReader("data"))
	req.RemoteAddr = "127.0.0.1:44444"
	req.Header.Set("X-Tab-Token-Create", "1")
	w := httptest.NewRecorder()
	h.Handle()(w, req)
	// Updates an existing tab → 200 (not 201 and not protected)
	if w.Code != http.StatusOK {
		t.Errorf("POST to existing default: expected 200, got %d", w.Code)
	}
	if w.Header().Get("X-Generated-Token") != "" {
		t.Error("default tab should NOT receive a generated token")
	}

	// GET without token → still works
	req2 := httptest.NewRequest(http.MethodGet, "/clipboard", nil)
	w2 := httptest.NewRecorder()
	h.Handle()(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("GET default after attempted protection: expected 200, got %d", w2.Code)
	}
}
func TestClipboardConcurrentReadWrite(t *testing.T) {
	h := NewClipboardHandler(true, 50)
	handle := h.Handle()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				req := httptest.NewRequest(http.MethodPost, "/clipboard", strings.NewReader("data"))
				req.RemoteAddr = "10.0.0.1:10000"
				w := httptest.NewRecorder()
				handle(w, req)
			} else {
				req := httptest.NewRequest(http.MethodGet, "/clipboard", nil)
				w := httptest.NewRecorder()
				handle(w, req)
			}
		}(i)
	}
	wg.Wait()
}
