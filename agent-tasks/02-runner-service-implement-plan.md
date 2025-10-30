# Plan: Runner Service (Go) — Clone → OpenTofu Plan → PR Comment

Note
- This task describes a pull-subscriber runner. It has been superseded by Task 03, which migrates to a push-based HTTP runner on Cloud Run. See `agent-tasks/03-runner-service-push-delivery.md` for the current architecture and wiring. The plan execution and commenting details here still apply conceptually.

Goal
- Implement a minimal runner on Cloud Run (us-east4) that consumes `plan` jobs from Pub/Sub, shallow‑clones the repo at the PR head SHA, runs OpenTofu plan, and posts/updates a PR comment with the plan output.

Scope (historical)
- Pull from Pub/Sub subscription `plan-runner`.
- Auth to GitHub via installation token provided in message.
- Local state (no remote backend). No provider cache optimization yet.
- Post or update a single comment per PR using a stable marker.

Assumptions
- App service will publish messages to topic `plan` including a valid installation access token and required fields.
- Region/project: `us-east4` / `devstage-419614`.

Message schema (input)
- Minimal required fields:
  - `repo.full_name` (e.g., `owner/repo`), `repo.clone_url`
  - `pull_request.number`, `pull_request.head_sha`
  - `installation.token` (GitHub App installation access token)
  - `work.dir` (default `.`), `plan_id` (unique per PR head)
  - `github_api_base_url` (default `https://api.github.com`)

Architecture/Choices
- Language: Go 1.25.
- Container: Debian slim with `git`, `ca-certificates`, OpenTofu installed at build time.
- Concurrency: 1–4 messages in parallel; ack on success, nack on retriable errors.
- Clone strategy: token in HTTPS URL, fetch `--depth=1` for the specific SHA, safe.directory set.
- Plan execution: `tofu init -no-color`, `tofu plan -no-color -out tfplan.bin`, `tofu show -no-color tfplan.bin`.
- Commenting: REST call to `POST /repos/{owner}/{repo}/issues/{number}/comments`; update existing if body contains marker `<!-- runners-poc:plan:{plan_id} -->`.
- Logging: structured JSON to stdout; redact tokens.

Deliverables
- `/runner` Go service with:
  - Pub/Sub consumer, health endpoint `/healthz`.
  - Git clone + OpenTofu plan executor.
  - GitHub PR comment create/update.
- Dockerfile, minimal `deploy.sh` (build/push/deploy) similar to app.

Step-by-step
1) Scaffold Go service
- Module init, main with config (env), logger, `/healthz`.

2) Pub/Sub receive loop
- Pull from `PUBSUB_SUBSCRIPTION_PLAN` (`plan-runner`), JSON decode, per‑message handler with context deadline.

3) Validate and prepare workdir
- Create temp dir, compute absolute module path = repo root + `work.dir`.

4) Shallow clone at SHA
- `git init`, add remote with token in URL `https://x-access-token:<token>@github.com/owner/repo.git`.
- `git fetch --depth=1 origin <head_sha>`; `git checkout FETCH_HEAD`.
- `git config --global --add safe.directory <repoDir>`.

5) Run OpenTofu
- In module path: `tofu init -input=false -no-color`.
- `tofu plan -input=false -no-color -out=tfplan.bin`.
- `tofu show -no-color tfplan.bin` → capture output string (truncate if huge, e.g., 200KB).

6) Post/Update PR comment
- Build body:
  - Header with repo and commit.
  - Marker `<!-- runners-poc:plan:{plan_id} -->`.
  - Fenced code block with plan output.
- List existing comments; if marker found → PATCH; else → POST.

7) Ack/Nack and cleanup
- On success: ack. On transient error (GitHub 5xx, rate limit) → nack with backoff. Always cleanup temp dir.

8) Container & deploy (historical)
- Dockerfile installs `git`, `curl`, `ca-certificates`, and OpenTofu Linux amd64.
- Env: `PUBSUB_SUBSCRIPTION_PLAN=plan-runner`, `GITHUB_API_BASE_URL` (optional).
- Deploy Cloud Run `runner` in `us-east4` with service account having `roles/pubsub.subscriber`.

9) Validate
- Manually publish a test message to topic `plan` (or wire from app) and verify a PR comment appears/updates.

Env vars
- `PUBSUB_SUBSCRIPTION_PLAN`: name of subscription (e.g., `plan-runner`).
- `GITHUB_API_BASE_URL`: default `https://api.github.com`.
- `PORT`: provided by Cloud Run.

IAM
- Runner service account: `roles/pubsub.subscriber` on `plan-runner`.
- Artifact Registry reader to pull image.

Acceptance
- Given a valid message for an installed repo/PR, runner posts a single sticky comment with the OpenTofu plan and updates it on subsequent runs for the same `plan_id`.

Next (after MVP)
- Publish from App service on PR events.
- Basic provider cache (bake GCP provider into image).
- Coalesce multiple synchronize events per PR head.
