package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPError represents a non-2xx HTTP response returned by the remote service.
type HTTPError struct {
	StatusCode int
	Body       []byte
	Header     http.Header
	JSON       any
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("http error: status=%d body=%s", e.StatusCode, string(e.Body))
}

// Retryable reports whether the error should be considered transient.
func (e *HTTPError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode == http.StatusRequestTimeout ||
		(e.StatusCode >= 500 && e.StatusCode <= 599)
}

// decodeJSONBody parses the body bytes into a generic JSON payload.
func decodeJSONBody(body []byte) any {
	if len(body) == 0 {
		return nil
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	return payload
}
