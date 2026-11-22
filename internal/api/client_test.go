package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(url string) *Client {
	c := New("test-key")
	c.BaseURL = url
	c.PollMin, c.PollMax = time.Millisecond, time.Millisecond
	return c
}

func TestDoPollsUntilSucceeded(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Path != "/v9/people" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		switch calls.Add(1) {
		case 1:
			if !strings.Contains(string(body), `"people"`) {
				t.Errorf("kickoff body = %s", body)
			}
			io.WriteString(w, `{"request_id":"r1","status":"pending","credits_used":0,"credits_remaining":100}`)
		case 2:
			if string(body) != `{"request_id":"r1"}` {
				t.Errorf("poll body = %s", body)
			}
			io.WriteString(w, `{"request_id":"r1","status":"running"}`)
		default:
			io.WriteString(w, `{"request_id":"r1","status":"succeeded","credits_used":5,"people":[]}`)
		}
	}))
	defer srv.Close()

	resp, err := testClient(srv.URL).People(context.Background(), PeopleRequest{
		People: []PersonInput{{Name: "A", CompanyName: "B"}},
		Select: Select{Attributes: Attrs([]string{"name"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
	if resp.Status != "succeeded" || *resp.CreditsUsed != 5 || *resp.InitialCreditsRemaining != 100 {
		t.Errorf("unexpected response %+v", resp)
	}
}

func TestDoFailedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"request_id":"r1","status":"failed","error":{"code":"unavailable","message":"try later","retry_at":"2026-01-01T00:00:00Z"}}`)
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Do(context.Background(), "/v9/people", map[string]any{})
	var je *JobError
	if !errors.As(err, &je) || je.Code != "unavailable" || je.Message != "try later" {
		t.Fatalf("err = %v", err)
	}
}

func TestRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"status":"succeeded","organizations":[]}`)
	}))
	defer srv.Close()

	if _, err := testClient(srv.URL).Do(context.Background(), "/v9/organizations", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestHTTPErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]any{
			"detail": []any{map[string]any{"loc": []any{"body", "people", 0}, "msg": "name requires company"}},
		})
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Do(context.Background(), "/v9/people", map[string]any{})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 422 {
		t.Fatalf("err = %v", err)
	}
	if want := "body.people.0: name requires company"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to contain %q", err, want)
	}
}

func TestDoTimesOutWhilePolling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"request_id":"r1","status":"pending"}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := testClient(srv.URL).Do(ctx, "/v9/people", map[string]any{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}
