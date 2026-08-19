# pf-developer-ci-dash

P11 CI pipeline dashboard (idea 15). **Read-only** view of GitHub Actions on **allowlisted public repositories**. No PAT in git. Scanner is not this repo and is not on the K8s overlay.

Learning / portfolio sample. Formal docs: `project/portfolio-plan/developer-platform/docs/`.

## Demo

```powershell
go test ./...
$env:CI_DASH_REPOS = "oasdiff/oasdiff"
go run ./cmd/ci-dash
# http://localhost:8115
# http://localhost:8115/repos/oasdiff/oasdiff
```

Optional `GITHUB_TOKEN` (read-only Actions) only raises the unauthenticated rate limit. Keep it in the environment, never in the repo.

Optional webhook (HMAC): set `GITHUB_WEBHOOK_SECRET` and point GitHub at `POST /webhook/github` for `workflow_run`. Unsigned webhooks are 404.

## Compose

```powershell
copy deploy\.env.example deploy\.env
docker compose -f deploy\compose.yaml --env-file deploy\.env up --build
```

## Not in this slice

Private repos, PAT committed to git, self-hosted runner execution, flake charts, PostgreSQL history, K8s overlay.
