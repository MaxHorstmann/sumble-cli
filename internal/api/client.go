// Package api is a minimal client for the Sumble REST API (v9).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API host.
const DefaultBaseURL = "https://api.sumble.com"

// Client talks to the Sumble API. Create one with New.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	PollMin    time.Duration // first delay between polls of an async request
	PollMax    time.Duration // cap for the (doubling) poll delay
	MaxRetries int           // retries on HTTP 429
	UserAgent  string
}

// New returns a Client with production defaults.
func New(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		// The server gives up after 55s (HTTP 504), so allow a bit more per call.
		HTTP:       &http.Client{Timeout: 65 * time.Second},
		PollMin:    time.Second,
		PollMax:    5 * time.Second,
		MaxRetries: 3,
		UserAgent:  "sumble-cli",
	}
}

// Response is the common envelope of people/organizations responses.
// Raw holds the full JSON body of the final response.
type Response struct {
	Raw              []byte          `json:"-"`
	RequestID        string          `json:"request_id"`
	Status           string          `json:"status"`
	CreditsUsed      *float64        `json:"credits_used"`
	CreditsRemaining *float64        `json:"credits_remaining"`
	Error            json.RawMessage `json:"error"`

	// InitialCreditsRemaining is credits_remaining as reported by the first
	// (kickoff) call, before any credits were charged.
	InitialCreditsRemaining *float64 `json:"-"`
}

// HTTPError is a non-2xx response from the API.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	var hint string
	switch e.Status {
	case http.StatusUnauthorized:
		hint = "invalid API key, check SUMBLE_API_KEY"
	case http.StatusPaymentRequired:
		hint = "insufficient credits"
	case http.StatusForbidden:
		hint = "forbidden, this feature may require a Pro or Enterprise plan"
	case http.StatusTooManyRequests:
		hint = "rate limited"
	case http.StatusGatewayTimeout:
		hint = "server timed out (no credits charged)"
	}
	msg := fmt.Sprintf("HTTP %d", e.Status)
	if hint != "" {
		msg += " " + hint
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// JobError is an async request that finished with status "failed".
type JobError struct {
	RequestID string
	Code      string
	Message   string
	RetryAt   string
}

func (e *JobError) Error() string {
	msg := "request failed"
	if e.Code != "" {
		msg += " (" + e.Code + ")"
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.RetryAt != "" {
		msg += "; retry at " + e.RetryAt
	}
	return msg
}

// Do POSTs body to path. If the API answers with a pending/running async
// request, Do polls it until it succeeds, fails, or ctx is done.
func (c *Client) Do(ctx context.Context, path string, body any) (*Response, error) {
	resp, err := c.call(ctx, path, body)
	if err != nil {
		return nil, err
	}
	initial := resp.CreditsRemaining
	delay := c.PollMin
	for resp.Status == "pending" || resp.Status == "running" {
		id := resp.RequestID
		if id == "" {
			return nil, fmt.Errorf("API returned status %q without a request_id", resp.Status)
		}
		if err := sleep(ctx, delay); err != nil {
			return nil, fmt.Errorf("waiting for request %s: %w", id, err)
		}
		delay = min(delay*2, c.PollMax)
		resp, err = c.call(ctx, path, map[string]string{"request_id": id})
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("waiting for request %s: %w", id, ctx.Err())
			}
			return nil, err
		}
	}
	resp.InitialCreditsRemaining = initial
	if resp.Status == "failed" {
		return nil, newJobError(resp)
	}
	return resp, nil
}

func (c *Client) call(ctx context.Context, path string, body any) (*Response, error) {
	data, err := c.post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	r.Raw = data
	return &r, nil
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			if err := sleep(ctx, retryAfter(resp.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, &HTTPError{Status: resp.StatusCode, Message: errorMessage(data)}
		}
		return data, nil
	}
}

func newJobError(r *Response) error {
	e := &JobError{RequestID: r.RequestID}
	var obj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		RetryAt any    `json:"retry_at"`
	}
	if err := json.Unmarshal(r.Error, &obj); err == nil {
		e.Code, e.Message = obj.Code, obj.Message
		if obj.RetryAt != nil {
			e.RetryAt = fmt.Sprint(obj.RetryAt)
		}
	} else {
		var s string
		if json.Unmarshal(r.Error, &s) == nil {
			e.Message = s
		}
	}
	return e
}

// errorMessage extracts a human-readable message from an error body.
func errorMessage(data []byte) string {
	var obj map[string]any
	if json.Unmarshal(data, &obj) == nil {
		for _, k := range []string{"detail", "message", "error"} {
			if s := describe(obj[k]); s != "" {
				return s
			}
		}
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

func describe(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		msg, _ := x["msg"].(string)
		if loc, ok := x["loc"].([]any); ok && msg != "" {
			parts := make([]string, len(loc))
			for i, p := range loc {
				parts[i] = fmt.Sprint(p)
			}
			return strings.Join(parts, ".") + ": " + msg
		}
		for _, k := range []string{"message", "msg", "detail"} {
			if s, ok := x[k].(string); ok && s != "" {
				return s
			}
		}
	case []any:
		var parts []string
		for _, item := range x {
			if s := describe(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	}
	return ""
}

func retryAfter(h string, attempt int) time.Duration {
	h = strings.TrimSpace(h)
	d := time.Second << attempt
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		d = time.Until(t)
	}
	return max(0, min(d, time.Minute))
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
