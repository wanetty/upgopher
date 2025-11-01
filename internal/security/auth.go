package security

import (
	"crypto/subtle"
	"net/http"
)

// ApplyBasicAuth wraps a handler with basic authentication
func ApplyBasicAuth(handler http.HandlerFunc, user, pass string) http.HandlerFunc {
	userByte := []byte(user)
	passByte := []byte(pass)
	return basicAuth(handler, userByte, passByte)
}

// basicAuth performs constant-time authentication check to prevent timing attacks
func basicAuth(handler http.HandlerFunc, username, password []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), username) != 1 || subtle.ConstantTimeCompare([]byte(pass), password) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized.\n"))
			return
		}
		handler(w, r)
	}
}
