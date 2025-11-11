package proxy

import "net/http"

// livenessHandler handles liveness probe requests.
// Always returns 200 OK to indicate the process is alive.
func livenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
	}
}

// readinessHandler handles readiness probe requests.
// Returns 200 OK if the application is ready to serve traffic, 503 otherwise.
func readinessHandler(checker ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if checker.IsReady() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}
}
