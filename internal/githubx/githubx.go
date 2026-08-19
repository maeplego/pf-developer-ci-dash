package githubx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var repoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,38}/[A-Za-z0-9._-]{1,100}$`)

type Client struct {
	BaseURL   string
	Token     string
	UserAgent string
	HTTP      *http.Client
	Allow     map[string]struct{}
}

func New(baseURL, token, userAgent string, allow []string) *Client {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	if userAgent == "" {
		userAgent = "pf-developer-ci-dash"
	}
	m := map[string]struct{}{}
	for _, r := range allow {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		m[r] = struct{}{}
	}
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		UserAgent: userAgent,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		Allow:     m,
	}
}

func ParseAllowlist(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, _, err := SplitRepo(p); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

func SplitRepo(s string) (owner, name string, err error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")
	s = strings.TrimSuffix(s, ".git")
	s = strings.Trim(s, "/")
	if strings.Contains(s, "..") || strings.ContainsAny(s, `\:`) || strings.Count(s, "/") != 1 {
		return "", "", fmt.Errorf("invalid repository %q", s)
	}
	if !repoRe.MatchString(s) {
		return "", "", fmt.Errorf("invalid repository %q", s)
	}
	parts := strings.Split(s, "/")
	return parts[0], parts[1], nil
}

func (c *Client) Allowed(owner, name string) bool {
	_, ok := c.Allow[owner+"/"+name]
	return ok
}

func (c *Client) Get(path string) ([]byte, int, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, 0, fmt.Errorf("path must be absolute on api.github.com")
	}
	u := c.BaseURL + path
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return b, res.StatusCode, nil
}

func (c *Client) Post(path string, body any) ([]byte, int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, strings.NewReader(string(raw)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return b, res.StatusCode, err
}

func RepoAPIPath(owner, name, suffix string) (string, error) {
	if strings.Contains(suffix, "://") || strings.Contains(suffix, "..") {
		return "", fmt.Errorf("invalid api path")
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + suffix, nil
}

func ValidWebhookSignature(secret, header string, body []byte) bool {
	header = strings.TrimSpace(header)
	const p = "sha256="
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, p))
	if err != nil {
		return false
	}
	sum := hmac.New(sha256.New, []byte(secret))
	_, _ = sum.Write(body)
	return hmac.Equal(sum.Sum(nil), want)
}

type WorkflowRuns struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type WorkflowRun struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	HeadBranch   string `json:"head_branch"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HTMLURL      string `json:"html_url"`
	DisplayTitle string `json:"display_title"`
	RunStartedAt string `json:"run_started_at"`
	UpdatedAt    string `json:"updated_at"`
	Actor        struct {
		Login string `json:"login"`
	} `json:"actor"`
}

type Pull struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"base"`
}

type PullFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

type PullComment struct {
	ID       int64  `json:"id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Body     string `json:"body"`
	HTMLURL  string `json:"html_url"`
	CommitID string `json:"commit_id"`
	User     struct {
		Login string `json:"login"`
	} `json:"user"`
}
