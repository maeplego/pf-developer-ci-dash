# pf-developer-ci-dash

学習用の CI ダッシュボードです。許可した **公開** GitHub リポジトリの Actions を読むだけです。PAT を Git に置きません。**本番のパイプライン基盤ではありません。**

```powershell
go test ./...
$env:CI_DASH_REPOS = "oasdiff/oasdiff"
go run ./cmd/ci-dash
```

- 一覧: http://localhost:8115
- 例: http://localhost:8115/repos/oasdiff/oasdiff

任意の `GITHUB_TOKEN`（読み取り専用）は、未認証のレート制限を上げるためだけです。環境変数に置き、リポジトリには入れないでください。

Webhook（HMAC）を使うときは `GITHUB_WEBHOOK_SECRET` を設定し、GitHub から `POST /webhook/github`（`workflow_run`）を向けます。署名無しは 404 です。

プライベートリポジトリ、セルフホスト runner の実行、履歴用 Postgres はありません。Compose は `deploy/` です。
