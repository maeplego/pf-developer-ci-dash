package main

import (
	"log"
	"net/http"
	"os"

	"github.com/portfolio/pf-developer-ci-dash/internal/githubx"
	"github.com/portfolio/pf-developer-ci-dash/internal/httpapi"
)

func main() {
	addr := getenv("CI_DASH_HTTP_ADDR", ":8115")
	allow := githubx.ParseAllowlist(getenv("CI_DASH_REPOS", "oasdiff/oasdiff"))
	if len(allow) == 0 {
		log.Fatal("CI_DASH_REPOS must list public owner/name entries (comma-separated). Do not put a PAT in git.")
	}
	gh := githubx.New(os.Getenv("GITHUB_API_BASE"), os.Getenv("GITHUB_TOKEN"), "pf-developer-ci-dash", allow)
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	log.Printf("ci-dash listening on %s allow=%v (token set=%v)", addr, allow, os.Getenv("GITHUB_TOKEN") != "")
	if err := http.ListenAndServe(addr, httpapi.New(gh, secret)); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
