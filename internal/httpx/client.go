package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the HTTP client used by the helper.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithHeaders assigns default headers added to every request.
func WithHeaders(h http.Header) Option {
	return func(c *Client) {
		for k, values := range h {
			for _, v := range values {
				c.headers.Add(k, v)
			}
		}
	}
}

// Client wraps http.Client providing retry and base URL utilities.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	headers    http.Header
}

// Request describes a single outbound request.
type Request struct {
	Method       string
	Path         string
	Query        url.Values
	Header       http.Header
	DisableRetry bool
	Body         io.Reader
	GetBody      func() (io.ReadCloser, error)
}

// NewClient creates a Client for the provided base URL.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("httpx: base URL is required")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("httpx: invalid base URL: %w", err)
	}

	c := &Client{
		baseURL: parsed,
		httpClient: &http.Client{
			Timeout: 100 * time.Second,
		},
		headers: make(http.Header),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Do executes the provided request and returns the response, or an HTTPError.
func (c *Client) Do(ctx context.Context, req *Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("httpx: request is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Ensure body replay configuration is sane.
	if req.Method == "" {
		return nil, errors.New("httpx: HTTP method is required")
	}

	if req.DisableRetry {
		req.GetBody = nil
	} else if req.GetBody == nil && req.Body == nil {
		// no body is OK
	} else if req.GetBody == nil && req.Body != nil {
		// Attempt to buffer the body for retries.
		data, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("httpx: read request body: %w", err)
		}
		reader := bytes.NewReader(data)
		req.Body = reader
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
	}

	fullURL, err := c.buildURL(req.Path, req.Query)
	if err != nil {
		return nil, err
	}

	body, err := c.prepareBody(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL, body)
	if err != nil {
		return nil, err
	}

	httpReq.Header = cloneHeader(c.headers)
	for k, values := range req.Header {
		for _, v := range values {
			httpReq.Header.Add(k, v)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		closeBody(respBody(resp))
		return nil, err
	}

	if resp.StatusCode >= 400 {
		err = c.handleError(resp)
		return nil, err
	}

	return resp, nil
}

func (c *Client) prepareBody(req *Request) (io.ReadCloser, error) {
	if req.Body != nil {
		body := req.Body
		req.Body = nil
		if rc, ok := body.(io.ReadCloser); ok {
			return rc, nil
		}
		return io.NopCloser(body), nil
	}
	return http.NoBody, nil
}

func closeBody(rc io.ReadCloser) {
	if rc != nil {
		_ = rc.Close()
	}
}

func respBody(resp *http.Response) io.ReadCloser {
	if resp == nil {
		return nil
	}
	return resp.Body
}

func (c *Client) buildURL(path string, q url.Values) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	if len(q) > 0 {
		ref.RawQuery = q.Encode()
	}
	full := c.baseURL.ResolveReference(ref)
	return full.String(), nil
}

func (c *Client) handleError(resp *http.Response) error {
	defer closeBody(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("httpx: read error body: %w", err)
	}
	httpErr := &HTTPError{
		StatusCode: resp.StatusCode,
		Body:       body,
		Header:     resp.Header.Clone(),
	}
	if isJSON(resp.Header.Get("Content-Type")) {
		httpErr.JSON = decodeJSONBody(body)
	}
	return httpErr
}

// WithJSONBody serializes the supplied value into JSON and returns a reusable reader.
func WithJSONBody(v any) (io.Reader, string, error) {
	data, err := jsonMarshal(v)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(data), "application/json", nil
}

// ReadAllAndClose drains the reader and ensures it is closed.
func ReadAllAndClose(rc io.ReadCloser) ([]byte, error) {
	defer closeBody(rc)
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func isJSON(contentType string) bool {
	if contentType == "" {
		return false
	}
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}
	return strings.TrimSpace(contentType) == "application/json"
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, values := range src {
		vCopy := make([]string, len(values))
		copy(vCopy, values)
		dst[k] = vCopy
	}
	return dst
}

func jsonMarshal(v any) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	data := bytes.TrimRight(buf.Bytes(), "\n")
	return data, nil
}
