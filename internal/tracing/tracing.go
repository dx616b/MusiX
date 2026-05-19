package tracing

import "net/http"

// HTTPRoundTripper is a no-op passthrough for the initial MusiX scaffold.
func HTTPRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		return http.DefaultTransport
	}
	return base
}
