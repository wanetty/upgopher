package middleware

import (
	"log"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.length += n
	return n, err
}

type statusWriterFlusher struct {
	*statusWriter
}

func (w statusWriterFlusher) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// AccessLog returns a middleware that logs details of each request.
// If quiet is true, it does not log anything.
func AccessLog(quiet bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if quiet {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			base := &statusWriter{ResponseWriter: w}
			var sw http.ResponseWriter = base

			// If the underlying ResponseWriter supports http.Flusher (like for SSE),
			// wrap it with statusWriterFlusher so assertions still pass.
			if _, ok := w.(http.Flusher); ok {
				sw = statusWriterFlusher{base}
			}

			next.ServeHTTP(sw, r)

			if base.status == 0 {
				base.status = http.StatusOK
			}

			duration := time.Since(start)
			log.Printf("[%s] [%s] %d %s %s (%s, %d bytes)\n",
				start.Format("2006-01-02 15:04:05"),
				r.Method,
				base.status,
				r.URL.String(),
				r.RemoteAddr,
				duration.Round(time.Microsecond),
				base.length,
			)
		})
	}
}
