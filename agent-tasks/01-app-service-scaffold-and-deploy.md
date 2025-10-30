# Plan: App Service Scaffold + Deploy (GitHub App Webhooks)

Goal
- Stand up the TypeScript app service on Cloud Run (us-east4) that verifies GitHub webhooks and logs pull_request opened events. No DB unless strictly needed.

Out of Scope (for this step)
- Pub/Sub publishing, runner integration, PR comments, apply/plan logic.
- Database/persistence (unless we decide to dedupe deliveries persistently).

Architecture/Choices
- Runtime: Node.js 20, TypeScript.
- Framework: Express (simple, widespread) with `@octokit/webhooks` for signature validation.
- GitHub App auth: `@octokit/app` (JWT + installation token helpers; token not yet used in this step).
- Endpoint: `POST /webhook` handles all GitHub events, filters `pull_request` with `action=opened` (and can log synchronize/reopened for future).
- Health: `GET /healthz` returns 200 for Cloud Run health checks.
- Logs: structured JSON to stdout (Cloud Run Logging).
- Security: Require HMAC signature; allow unauthenticated ingress (GitHub needs public access). Secrets via Secret Manager.

Deliverables
- `/app` service scaffold (TS): minimal server, webhook verification, logging for PR opened.
- Dockerfile + .dockerignore for `/app`.
- Cloud Run service in `us-east4` with env + secrets wired.
- GitHub App configured to send webhooks to Cloud Run URL.

Environment Variables
- `PORT`: provided by Cloud Run.
- `GITHUB_APP_ID`: numeric App ID.
- `GITHUB_WEBHOOK_SECRET` (secret): HMAC for webhook validation.
- `GITHUB_PRIVATE_KEY_PEM` (secret): GitHub App private key PEM content (single line or base64; we’ll handle plain PEM string env).
- `GITHUB_API_BASE_URL` (optional): default `https://api.github.com`.

Permissions
- Cloud Run service account: `roles/secretmanager.secretAccessor`, `roles/logging.logWriter`.
- GitHub App permissions (initial): Metadata Read, Pull requests Read (write not needed yet).

Step-by-Step
1) Scaffold code
- Init `/app` with TypeScript, `express`, `@octokit/webhooks`, `@octokit/app`, and basic logger.
- Add `src/server.ts` exposing `/healthz` and `/webhook` with signature verification.
- Add `npm scripts` for build/start, `tsconfig.json`.

2) Local sanity (optional)
- Run server locally; send a signed sample webhook payload using `@octokit/webhooks` CLI or a small script.

3) Containerize
- Add Dockerfile using `node:20-slim`; multi-stage build to compile TS -> JS, run as non-root.
- Add `.dockerignore`.

4) Secrets and IAM
- Create secrets: `GITHUB_WEBHOOK_SECRET`, `GITHUB_PRIVATE_KEY_PEM` in Secret Manager.
- Create service account `app-service-sa` with Secret Manager accessor and Logging writer roles.

5) Build & Push
- Create Artifact Registry repo: `containers` in `us-east4` (once per project).
- Build image from `/app` context and push to `us-east4-docker.pkg.dev/$PROJECT/containers/app:TAG`.

6) Deploy Cloud Run (us-east4)
- Deploy service `app` with `--allow-unauthenticated`, set env vars and secret mappings, set service account.

7) Configure GitHub App
- Create a GitHub App or edit existing: set Webhook URL to `https://<cloud-run-url>/webhook` and Webhook Secret to match Secret Manager value.
- Grant permissions: Metadata (Read), Pull Requests (Read). Subscribe to Pull request events.
- Install the app on target repo/org.

8) Verify end-to-end
- Open a PR in the repo; verify Cloud Run logs show structured entry with repo/PR number/sha.
- Confirm 2xx responses in GitHub Delivery logs.

Commands/Examples
- Artifact Registry (one-time)
  - `gcloud artifacts repositories create containers --location=us-east4 --repository-format=docker`
  - `gcloud auth configure-docker us-east4-docker.pkg.dev`

- Secrets
  - `gcloud secrets create GITHUB_WEBHOOK_SECRET --replication-policy=automatic`
  - `printf "<secret>" | gcloud secrets versions add GITHUB_WEBHOOK_SECRET --data-file=-`
  - `gcloud secrets create GITHUB_PRIVATE_KEY_PEM --replication-policy=automatic`
  - `printf "%s" "$(cat app.private-key.pem)" | gcloud secrets versions add GITHUB_PRIVATE_KEY_PEM --data-file=-`

- Build & Push (from repo root)
  - `docker build -t us-east4-docker.pkg.dev/$GCP_PROJECT/containers/app:$(git rev-parse --short HEAD) -f app/Dockerfile app`
  - `docker push us-east4-docker.pkg.dev/$GCP_PROJECT/containers/app:$(git rev-parse --short HEAD)`

- Deploy Cloud Run
  - `gcloud run deploy app \
      --region=us-east4 \
      --image=us-east4-docker.pkg.dev/$GCP_PROJECT/containers/app:$(git rev-parse --short HEAD) \
      --allow-unauthenticated \
      --service-account=app-service-sa@${GCP_PROJECT}.iam.gserviceaccount.com \
      --set-env-vars=GITHUB_APP_ID=<app-id>,GITHUB_API_BASE_URL=https://api.github.com \
      --set-secrets=GITHUB_WEBHOOK_SECRET=GITHUB_WEBHOOK_SECRET:latest,GITHUB_PRIVATE_KEY_PEM=GITHUB_PRIVATE_KEY_PEM:latest`

Acceptance Criteria
- Cloud Run service reachable at `/webhook`; rejects bad signatures with 401; returns 200 for valid deliveries.
- On PR opened, logs a structured line with `event=pull_request`, `action=opened`, `repo`, `pr_number`, `head_sha`.
- GitHub Delivery dashboard shows 2xx deliveries; Cloud Run logs show the event within seconds.
- No database required for this step.

Notes / Future Hooks
- We can later publish to Pub/Sub topic `plan` from the same handler once runner is ready.
- For idempotency, if we see duplicate delivery GUIDs, we can add a memory cache now; if persistence is needed later, consider Supabase (Postgres) with a unique constraint on `delivery_guid`.
