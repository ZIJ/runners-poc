# AGENTS Guide: Managed Runners POC (OpenTofu)

This document guides contributors and automation on how this POC is structured and how to extend it. The goal is to build a minimal end‑to‑end system: a GitHub App receives PR events, enqueues a plan job on Pub/Sub, and a runner on Cloud Run executes OpenTofu and comments the plan back on the PR.

## High‑Level Architecture

- GitHub App service (`/app`, TypeScript)
  - Receives `pull_request` webhooks (opened, synchronized, reopened).
  - Determines target working directory(s) to plan (start with repo root for POC).
  - Creates a message for Pub/Sub with repo + PR details and an installation access token.
  - Publishes to `tofu-plan-requests` topic.

- Runner service (`/runner`, Go)
  - Subscribes to `tofu-plan-requests`.
  - Checks out the repository at the specified ref.
  - Runs `opentofu init` and `opentofu plan` (no-color), captures output.
  - Posts or updates a PR comment with the plan result.

- Google Cloud Run (region `us-east4`)
  - Hosts both services as separate deployments.
  - Uses dedicated service accounts with least-privilege IAM.

- Google Pub/Sub
  - Topic: `plan` (App → Runner).
  - Subscription: `plan-runner` (Runner pulls).

## Event Flow

1. GitHub → Webhook: `pull_request` event (opened/synchronize/reopen).
2. App service validates webhook signature and fetches installation access token.
3. App service publishes a `PlanRequest` to Pub/Sub topic `plan`.
4. Runner service receives the message, clones the repo at the specified ref.
5. Runner executes OpenTofu init/plan, collects a human-readable plan output.
6. Runner posts/updates a PR comment with the plan result, links to logs if helpful.

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
- For the POC, `work.dir` is the repo root (`.`). Multi-dir support can follow.
- The App service is responsible for including a valid installation token to avoid the runner having to mint one.

## Topics and Subscriptions

- Topic: `plan`
- Subscription: `plan-runner`

Create these during infra bootstrap or at deploy time.

## Environments and Secrets

Shared
- `GCP_PROJECT_ID`: GCP project holding Pub/Sub and Cloud Run.
- `RUNTIME_REGION`: `us-east4`.

App service (TypeScript)
- `GITHUB_APP_ID`: Numeric GitHub App ID.
- `GITHUB_WEBHOOK_SECRET`: HMAC secret for webhook signature verification.
- `GITHUB_PRIVATE_KEY_PEM`: PEM contents of the GitHub App private key.
- `PUBSUB_TOPIC_PLAN`: `plan`.
- `GITHUB_API_BASE_URL` (optional): default `https://api.github.com`.
- Access to Secret Manager is recommended for sensitive values.

Runner service (Go)
- `PUBSUB_SUBSCRIPTION_PLAN`: `plan-runner`.
- `GITHUB_API_BASE_URL` (optional): default `https://api.github.com`.
- `TOFU_VERSION` (optional): pin OpenTofu version or use image default.
- The runner expects an `installation.token` in each message for checkout and commenting.

## IAM and Permissions

- App service account
  - `roles/pubsub.publisher` on topic `plan`.
  - `roles/secretmanager.secretAccessor` for GitHub secrets.
  - `roles/logging.logWriter` for structured logs.

- Runner service account
  - `roles/pubsub.subscriber` on subscription `plan-runner`.
  - `roles/secretmanager.secretAccessor` if secrets are needed.
  - `roles/logging.logWriter`.

GitHub App permissions
- Pull requests: Read & write (post comments).
- Contents: Read (clone via token).
- Metadata: Read.
- Checks: Write (optional, future for richer reporting).

## OpenTofu Execution (Runner)

Minimal POC flow per message:
- Clone: `git clone <clone_url> && git checkout <head_sha>`.
- Working dir: `work.dir` from message (default `.`).
- Init: `tofu init -input=false -no-color`.
- Plan: `tofu plan -input=false -no-color -out=tfplan.bin`.
- Show: `tofu show -no-color tfplan.bin` → capture string.
- Comment: Post a sticky PR comment identified by `plan_id` (update if it exists).

Notes
- Prefer `--no-color` to keep comments readable.
- If `tofu.lock.hcl` exists, init will pin providers. Backend auth is out of scope for POC; plan may use local state.
- For multi-module repos, later detect subpaths or use a config file (e.g., `.tofu-managed.yaml`).

## Commenting Strategy

- Create or update a single “Managed Runners (OpenTofu) Plan” comment per PR head SHA.
- Include commit SHA, working directory, and a collapsible code block with the plan output.
- Truncate if excessively large and link to logs (future).

## Local Development

Repo layout (proposed)
- `/app`: GitHub App service (TypeScript, Express/Fastify + Octokit).
- `/runner`: Runner service (Go, Pub/Sub + GitHub API + OpenTofu CLI).
- `/infra` (optional): IaC for topics, subscriptions, and Cloud Run services.

Useful tooling
- Pub/Sub emulator: `gcloud beta emulators pubsub start` and set `PUBSUB_EMULATOR_HOST`.
- Webhook tunneling: use your preferred tunnel to expose `/webhook` locally.

Dev loop (suggested)
- Run the App locally, receive a real or simulated PR webhook, publish to emulator.
- Run the Runner locally, pull from emulator, execute plan with a local OpenTofu install.

## Deployment (Cloud Run, `us-east4`)

- Build and push containers for `/app` and `/runner`.
- Create Pub/Sub topic and subscription.
- Deploy both services to Cloud Run with appropriate env vars and service accounts.
- Verify webhook delivery → Pub/Sub publish → runner execution → PR comment.

## Future Optimizations

- Provider warm‑start: bake GCP/OpenTofu providers into the image to skip downloads.
- Git checkout cache: shallow clone + merge‑base diff to target only changed modules.
- Concurrency controls and per‑PR coalescing to avoid redundant runs.
- Rich reporting via the Checks API with artifacts.

## Out of Scope (for POC)

- Remote state backends and cloud credentials wiring.
- Multi‑workspace/module orchestration.
- Policy as code, drift detection, apply workflows.
