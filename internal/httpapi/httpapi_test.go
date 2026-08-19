package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portfolio/pf-developer-ci-dash/internal/githubx"
)

func fixtureGH(t *testing.T, status int, body string) (*httptest.Server, http.Handler) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("token must not be set in this fixture")
		}
		if r.URL.Path != "/repos/oasdiff/oasdiff/actions/runs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	gh := githubx.New(upstream.URL, "", "test", []string{"oasdiff/oasdiff"})
	gh.HTTP = upstream.Client()
	return upstream, New(gh, "whsec")
}

func TestHealthAndAllowlist(t *testing.T) {
	_, h := fixtureGH(t, 200, `{"total_count":0,"workflow_runs":[]}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/repos/octocat/Hello-World", nil))
	if rr.Code != 403 {
		t.Fatalf("want 403 got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/repos/oasdiff/oasdiff?path=C:/git/secret", nil))
	if rr.Code != 400 {
		t.Fatalf("path query %d %s", rr.Code, rr.Body.String())
	}
}

func TestRunsJSON(t *testing.T) {
	payload := `{"total_count":1,"workflow_runs":[{"id":1,"name":"test","head_branch":"main","event":"push","status":"completed","conclusion":"failure","html_url":"https://github.com/oasdiff/oasdiff/actions/runs/1","display_title":"ci","run_started_at":"2026-08-19T00:00:00Z","updated_at":"2026-08-19T00:01:00Z","actor":{"login":"octocat"}}]}`
	_, h := fixtureGH(t, 200, payload)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/repos/oasdiff/oasdiff/runs", nil))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		ByWorkflow []struct {
			Name    string `json:"name"`
			Failure int    `json:"failure"`
		} `json:"by_workflow"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ByWorkflow) != 1 || body.ByWorkflow[0].Failure != 1 {
		t.Fatalf("%+v", body)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/repos/oasdiff/oasdiff", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "failure") {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}

func TestWebhook(t *testing.T) {
	_, h := fixtureGH(t, 200, `{"total_count":0,"workflow_runs":[]}`)
	body := []byte(`{"repository":{"full_name":"oasdiff/oasdiff"},"workflow_run":{"name":"test","conclusion":"failure"}}`)
	mac := hmac.New(sha256.New, []byte("whsec"))
	_, _ = mac.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 202 {
		t.Fatalf("%d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", "sha256=00")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("bad sig %d", rr.Code)
	}
}
