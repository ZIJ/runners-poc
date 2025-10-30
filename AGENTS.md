# AGENTS Guide: Managed Runners POC (OpenTofu)

This document guides contributors and automation on how this POC is structured. Target: GitHub App receives PR events, publishes a plan job (topic `plan`), and a runner on Cloud Run executes OpenTofu and comments the plan back on the PR.

## High‑Level Architecture

- GitHub App service (`/app`, TypeScript)
  - Receives `pull_request` webhooks (opened, synchronized, reopened) and logs events.
  - Validates webhook signatures (`GITHUB_WEBHOOK_SECRET`).
  - Planned next: publish to Pub/Sub topic `plan`.

- Runner service (`/runner`, Go)
  - Exposes `POST /pubsub/push` on Cloud Run (push delivery).
  - Decodes Pub/Sub envelope, executes OpenTofu plan, and comments results.

- Google Cloud Run (region `us-east4`)
  - Hosts both services as separate deployments.
  - Uses dedicated service accounts with least-privilege IAM.

- Google Pub/Sub
  - Topic: `plan` (App → Runner). Push subscription `plan-runner` delivers to Runner.

## Event Flow

1. GitHub → Webhook: events (ping, installation, pull_request).
2. App service validates webhook signature and logs key details.
3. App service publishes `PlanRequest` to Pub/Sub topic `plan`.
4. Runner (push) clones repo, runs OpenTofu plan, comments results.

## Pub/Sub Message Schema (PlanRequest)

```json
{
  "request_id": "uuid-v4",
  "repo": {
    "full_name": "owner/repo",
    "clone_url": "https://github.com/owner/repo.git",
    "default_branch": "main"
  },
  "pull_request": {
    "number": 123,
    "head_sha": "<commit-sha>",
    "head_ref": "feature/a",
    "base_ref": "main"
  },
  "installation": {
    "id": 12345678,
    "token": "<installation-access-token>",
    "expires_at": "2025-01-01T00:00:00Z"
  },
  "work": {
    "dir": ".", 
    "tofu_version": "1.7.x",
    "plan_id": "pr-123-<short-sha>"
  },
  "github_api_base_url": "https://api.github.com"
}
```

Notes:
- Start with `work.dir` at repo root (`.`). Multi-dir support can follow.
- App will include an installation token later when runner needs it.

## Topics and Subscriptions

- Topic: `plan`
- Push Subscription: `plan-runner` → `https://<runner-url>/pubsub/push`
  - Use OIDC push auth with a service account that has `roles/run.invoker` on the runner.

## Environment

- App service: `GITHUB_WEBHOOK_SECRET` (required). Env vars only for POC.
- Region: `us-east4`. GCP Project: devstage-419614.
- Runner: HTTP service, expects Pub/Sub push (no subscriber config). Uses GitHub API via installation token.

## Permissions

- Cloud Run service uses `app-service-sa@devstage-419614.iam.gserviceaccount.com`.
- It needs to pull from Artifact Registry (`artifactregistry.reader`).
- GitHub App: Permissions for now — Metadata (Read), Pull requests (Read). Install via GitHub UI.

## Runner Plan

Flow:
- Receive push, clone repo@SHA, `tofu init/plan/show`, post sticky PR comment.

Notes
- Prefer `--no-color` to keep comments readable.
- If `tofu.lock.hcl` exists, init will pin providers. Backend auth is out of scope for POC; plan may use local state.
- For multi-module repos, later detect subpaths or use a config file (e.g., `.tofu-managed.yaml`).

## Commenting Strategy

- Create or update a single “Managed Runners (OpenTofu) Plan” comment per PR head SHA.
- Include commit SHA, working directory, and a collapsible code block with the plan output.
- Truncate if excessively large and link to logs (future).

## Local Development

Repo layout
- `/app`: GitHub App service (TypeScript + Express + Octokit).
- `/runner`: Runner service (Go, push HTTP). See `runner/internal/*`.

Dev loop
- Run the App locally: `cd app && npm i && export GITHUB_WEBHOOK_SECRET=dev && npm run dev`.

## Deployment (App)

- Use `./app/deploy.sh` with `GITHUB_WEBHOOK_SECRET`, `GITHUB_APP_ID`, and `GITHUB_PRIVATE_KEY_PEM` exported. The script builds, pushes to Artifact Registry (`runners-poc`), and deploys to Cloud Run in `us-east4`.
- Set your GitHub App webhook to `<Cloud Run URL>/webhook` and use the same secret.

## Deployment (Runner)

- Deploy with `./runner/deploy.sh`. Create push subscription `plan-runner` to `<runner-url>/pubsub/push` with OIDC push auth. Grant `roles/run.invoker` to the Pub/Sub push service account on the runner.

## Future Optimizations

- Provider warm‑start: bake GCP/OpenTofu providers into the image to skip downloads.
- Git checkout cache: shallow clone + merge‑base diff to target only changed modules.
- Concurrency controls and per‑PR coalescing to avoid redundant runs.
- Rich reporting via the Checks API with artifacts.

## Out of Scope (for POC)

- Remote state backends and cloud credentials wiring.
- Multi‑workspace/module orchestration.
- Policy as code, drift detection, apply workflows.
