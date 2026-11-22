package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const personResponse = `{
  "request_id": "r1", "status": "succeeded", "credits_used": 5, "credits_remaining": 95,
  "people": [{
    "input": {"name": "Satya Nadella", "company_domain": "microsoft.com"},
    "person_id": 42,
    "sumble_url": "https://sumble.com/people/42",
    "attributes": {
      "name": "Satya Nadella",
      "job_title": "Chairman and CEO",
      "current_employer": {"organization_id": 7, "name": "Microsoft", "slug": "microsoft"},
      "location": "Redmond, US",
      "linkedin_url": "https://www.linkedin.com/in/example"
    }
  }]
}`

type result struct {
	code           int
	stdout, stderr string
}

// runCLI runs Main against a fake API that records the request body.
func runCLI(t *testing.T, response string, args ...string) (result, map[string]any) {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &body); err != nil {
			t.Errorf("bad request body %s", b)
		}
		io.WriteString(w, response)
	}))
	defer srv.Close()

	env := map[string]string{"SUMBLE_API_KEY": "k", "SUMBLE_API_BASE_URL": srv.URL}
	var out, errOut bytes.Buffer
	code := Main(context.Background(), args, func(k string) string { return env[k] }, &out, &errOut)
	return result{code, out.String(), errOut.String()}, body
}

func TestMissingKeyFailsImmediately(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Main(context.Background(), []string{"people", "find", "--name", "x", "--company", "y"},
		func(string) string { return "" }, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "SUMBLE_API_KEY is not set") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}

func TestHelpWorksWithoutKey(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"people", "find", "-h"}} {
		var out, errOut bytes.Buffer
		code := Main(context.Background(), args, func(string) string { return "" }, &out, &errOut)
		if code != 0 || out.Len()+errOut.Len() == 0 {
			t.Errorf("%v: code=%d", args, code)
		}
	}
}

func TestPeopleFindTable(t *testing.T) {
	res, body := runCLI(t, personResponse,
		"people", "find", "--name", "Satya Nadella", "--company-domain", "microsoft.com")
	if res.code != 0 {
		t.Fatalf("code=%d stderr=%q", res.code, res.stderr)
	}
	person := body["people"].([]any)[0].(map[string]any)
	if person["name"] != "Satya Nadella" || person["company_domain"] != "microsoft.com" {
		t.Errorf("request person = %v", person)
	}
	if _, ok := body["filter"]; ok {
		t.Error("find must not send a filter")
	}
	for _, want := range []string{"NAME", "JOB_TITLE", "SUMBLE_URL", "Satya Nadella", "Microsoft", "https://sumble.com/people/42"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
	if !strings.Contains(res.stderr, "credits used: 5, remaining: 95") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestPeopleFindJSON(t *testing.T) {
	res, _ := runCLI(t, personResponse, "people", "find", "--linkedin", "https://linkedin.com/in/x", "--json")
	var v map[string]any
	if res.code != 0 || json.Unmarshal([]byte(res.stdout), &v) != nil || v["status"] != "succeeded" {
		t.Fatalf("code=%d stdout=%q", res.code, res.stdout)
	}
}

func TestPeopleFindNoMatch(t *testing.T) {
	res, _ := runCLI(t, `{"status":"succeeded","credits_used":0,"people":[{"input":{"name":"Nobody"},"attributes":null}]}`,
		"people", "find", "--name", "Nobody", "--company", "Nowhere")
	if res.code != 1 || !strings.Contains(res.stderr, "no results") || res.stdout != "" {
		t.Fatalf("%+v", res)
	}
}

func TestPeopleFindValidation(t *testing.T) {
	res, _ := runCLI(t, personResponse, "people", "find", "--name", "Satya Nadella")
	if res.code != 2 || !strings.Contains(res.stderr, "--name requires --company") {
		t.Fatalf("%+v", res)
	}
}

func TestPeopleSearchInterspersedFlags(t *testing.T) {
	res, body := runCLI(t, personResponse,
		"people", "search", "--limit", "5", "go", "developers", "--org-id", "1,2", "--location", "US:Texas", "--attrs", "name")
	if res.code != 0 {
		t.Fatalf("code=%d stderr=%q", res.code, res.stderr)
	}
	filter := body["filter"].(map[string]any)
	if filter["search"] != "go developers" || body["limit"] != float64(5) {
		t.Errorf("body = %v", body)
	}
	if ids := filter["organization_ids"].([]any); len(ids) != 2 || ids[0] != float64(1) {
		t.Errorf("organization_ids = %v", ids)
	}
	if attrs := body["select"].(map[string]any)["attributes"].([]any); len(attrs) != 1 || attrs[0] != "name" {
		t.Errorf("attributes = %v", attrs)
	}
}

func TestOrgsFindFlatRows(t *testing.T) {
	res, body := runCLI(t, `{"organizations":[{"id":10441,"name":"Stripe","slug":"stripe","url":"stripe.com"}]}`,
		"orgs", "find", "--url", "stripe.com")
	if res.code != 0 {
		t.Fatalf("code=%d stderr=%q", res.code, res.stderr)
	}
	if org := body["organizations"].([]any)[0].(map[string]any); org["url"] != "stripe.com" {
		t.Errorf("body = %v", body)
	}
	if !strings.Contains(res.stdout, "10441") || !strings.Contains(res.stdout, "Stripe") {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestAllAttributes(t *testing.T) {
	res, body := runCLI(t, personResponse, "people", "find", "--person-id", "42", "--attrs", "all")
	if res.code != 0 || body["select"].(map[string]any)["attributes"] != "all" {
		t.Fatalf("code=%d body=%v", res.code, body)
	}
	if first := strings.Fields(res.stdout)[0]; first != "NAME" {
		t.Errorf("first column = %q", first)
	}
}
