package httpapi

import (
	"bytes"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portfolio/pf-developer-ci-dash/internal/githubx"
)

type Server struct {
	gh     *githubx.Client
	secret string
	mu     sync.Mutex
	hint   string
}

func New(gh *githubx.Client, webhookSecret string) http.Handler {
	s := &Server{gh: gh, secret: webhookSecret}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ready", s.health)
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /repos/{owner}/{repo}", s.repo)
	mux.HandleFunc("GET /api/repos/{owner}/{repo}/runs", s.apiRuns)
	mux.HandleFunc("POST /webhook/github", s.webhook)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("path") != "" {
		http.Error(w, "local git paths are not supported", http.StatusBadRequest)
		return
	}
	if repo := strings.TrimSpace(r.URL.Query().Get("repo")); repo != "" {
		owner, name, err := githubx.SplitRepo(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/repos/"+owner+"/"+name, http.StatusFound)
		return
	}
	var b strings.Builder
	b.WriteString(pageHead("CI dashboard"))
	b.WriteString(`<main><h1>CI dashboard</h1><p class="muted">Read-only GitHub Actions for an allowlisted public repository. No PAT in git. Optional <code>GITHUB_TOKEN</code> only raises the rate limit.</p>`)
	b.WriteString(`<form method="get" action="/"><label>owner/repo <input name="repo" value="`)
	b.WriteString(html.EscapeString(r.URL.Query().Get("repo")))
	b.WriteString(`" placeholder="oasdiff/oasdiff"></label> <button type="submit">Open</button></form>`)
	s.mu.Lock()
	hint := s.hint
	s.mu.Unlock()
	if hint != "" {
		b.WriteString(`<p class="muted">Last webhook: ` + html.EscapeString(hint) + `</p>`)
	}
	b.WriteString(`<h2>Allowlist</h2><ul>`)
	for repo := range s.gh.Allow {
		b.WriteString(`<li><a href="/repos/` + html.EscapeString(repo) + `">` + html.EscapeString(repo) + `</a></li>`)
	}
	b.WriteString(`</ul></main></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) repo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := s.checkRepo(w, r)
	if !ok {
		return
	}
	runs, status, err := s.loadRuns(owner, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if status == http.StatusNotFound {
		http.Error(w, "github repository or Actions data not found (public repos only without a token)", http.StatusNotFound)
		return
	}
	if status >= 400 {
		http.Error(w, "github API "+strconv.Itoa(status), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderRuns(owner+"/"+name, runs)))
}

func (s *Server) apiRuns(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := s.checkRepo(w, r)
	if !ok {
		return
	}
	runs, status, err := s.loadRuns(owner, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"repo":           owner + "/" + name,
		"total":          runs.TotalCount,
		"workflow_runs":  runs.WorkflowRuns,
		"by_workflow":    summarize(runs.WorkflowRuns),
	})
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	if s.secret == "" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	if !githubx.ValidWebhookSignature(s.secret, r.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if r.Header.Get("X-GitHub-Event") != "workflow_run" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var payload struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		WorkflowRun struct {
			Conclusion string `json:"conclusion"`
			Name       string `json:"name"`
		} `json:"workflow_run"`
	}
	_ = json.Unmarshal(body, &payload)
	s.mu.Lock()
	s.hint = payload.Repository.FullName + " " + payload.WorkflowRun.Name + " " + payload.WorkflowRun.Conclusion
	s.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) checkRepo(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if r.URL.Query().Get("path") != "" {
		http.Error(w, "local git paths are not supported", http.StatusBadRequest)
		return "", "", false
	}
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	full := owner + "/" + name
	if _, _, err := githubx.SplitRepo(full); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", "", false
	}
	if !s.gh.Allowed(owner, name) {
		http.Error(w, "repository is not on the allowlist", http.StatusForbidden)
		return "", "", false
	}
	return owner, name, true
}

func (s *Server) loadRuns(owner, name string) (githubx.WorkflowRuns, int, error) {
	p, err := githubx.RepoAPIPath(owner, name, "/actions/runs?per_page=30")
	if err != nil {
		return githubx.WorkflowRuns{}, 0, err
	}
	raw, status, err := s.gh.Get(p)
	if err != nil {
		return githubx.WorkflowRuns{}, status, err
	}
	var runs githubx.WorkflowRuns
	if status == 200 {
		if err := json.Unmarshal(raw, &runs); err != nil {
			return githubx.WorkflowRuns{}, status, err
		}
	}
	return runs, status, nil
}

type wfStat struct {
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failure int    `json:"failure"`
}

func summarize(runs []githubx.WorkflowRun) []wfStat {
	idx := map[string]int{}
	var out []wfStat
	for _, r := range runs {
		i, ok := idx[r.Name]
		if !ok {
			idx[r.Name] = len(out)
			out = append(out, wfStat{Name: r.Name})
			i = len(out) - 1
		}
		out[i].Total++
		switch r.Conclusion {
		case "success":
			out[i].Success++
		case "failure", "timed_out", "startup_failure":
			out[i].Failure++
		}
	}
	return out
}

func renderRuns(repo string, runs githubx.WorkflowRuns) string {
	var b strings.Builder
	b.WriteString(pageHead(repo))
	b.WriteString(`<main><p><a href="/">catalog</a></p><h1>` + html.EscapeString(repo) + `</h1>`)
	b.WriteString(`<p class="muted">Last ` + strconv.Itoa(len(runs.WorkflowRuns)) + ` of ` + strconv.Itoa(runs.TotalCount) + ` runs (GitHub Actions, public).</p>`)
	b.WriteString(`<h2>By workflow</h2><table><thead><tr><th>workflow</th><th>n</th><th>ok</th><th>red</th></tr></thead><tbody>`)
	for _, s := range summarize(runs.WorkflowRuns) {
		b.WriteString("<tr><td>" + html.EscapeString(s.Name) + "</td><td>" + strconv.Itoa(s.Total) + "</td><td>" + strconv.Itoa(s.Success) + "</td><td>" + strconv.Itoa(s.Failure) + "</td></tr>")
	}
	b.WriteString(`</tbody></table><h2>Runs</h2><table><thead><tr><th></th><th>workflow</th><th>branch</th><th>title</th><th>actor</th><th>duration</th></tr></thead><tbody>`)
	for _, r := range runs.WorkflowRuns {
		cls := r.Conclusion
		if cls == "" {
			cls = r.Status
		}
		b.WriteString(`<tr><td class="` + html.EscapeString(cls) + `">` + html.EscapeString(cls) + `</td>`)
		b.WriteString(`<td>` + html.EscapeString(r.Name) + `</td>`)
		b.WriteString(`<td>` + html.EscapeString(r.HeadBranch) + `</td>`)
		b.WriteString(`<td><a href="` + html.EscapeString(r.HTMLURL) + `">` + html.EscapeString(r.DisplayTitle) + `</a></td>`)
		b.WriteString(`<td>` + html.EscapeString(r.Actor.Login) + `</td>`)
		b.WriteString(`<td>` + html.EscapeString(duration(r.RunStartedAt, r.UpdatedAt)) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></main></body></html>`)
	return b.String()
}

func duration(start, end string) string {
	t0, err0 := time.Parse(time.RFC3339, start)
	t1, err1 := time.Parse(time.RFC3339, end)
	if err0 != nil || err1 != nil || t1.Before(t0) {
		return ""
	}
	return t1.Sub(t0).Truncate(time.Second).String()
}

func pageHead(title string) string {
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>`)
	buf.WriteString(html.EscapeString(title))
	buf.WriteString(` · pf-developer-ci-dash</title><style>
body{margin:0;font:15px/1.5 ui-sans-serif,system-ui,sans-serif;background:#0e1116;color:#e7ecf3}
main{padding:1.5rem 2rem;max-width:1100px}
a{color:#7dd3c7} .muted{color:#93a0b4} .err{color:#ff7b72}
table{border-collapse:collapse;width:100%}
td,th{border-bottom:1px solid #2b3340;padding:.4rem .5rem;text-align:left}
.success{color:#3dd68c;font-weight:700} .failure,.timed_out{color:#ff7b72;font-weight:700}
input{background:#161b22;color:#e7ecf3;border:1px solid #2b3340;padding:.3rem .5rem;border-radius:8px}
button{background:#212833;color:#e7ecf3;border:1px solid #2b3340;padding:.3rem .7rem;border-radius:8px}
</style></head><body>`)
	return buf.String()
}
