package proxy

import "net/http"

// Recovery recovers from panics in HTTP handlers and returns HTTP 500 to the client.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				// Logging of panics is handled in Logging middleware
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// RequestSizeLimit enforces maximum request body size.
// Handlers that read the body will receive *http.MaxBytesError when the limit is exceeded.
func RequestSizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// applyMiddlewares applies middlewares to a handler in the order they appear.
// The first middleware in the slice is the outermost (executes first).
func applyMiddlewares(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
