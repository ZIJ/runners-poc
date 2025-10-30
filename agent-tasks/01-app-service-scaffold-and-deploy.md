# Plan: App Service Scaffold + Deploy (GitHub App Webhooks)

Goal
- Deploy the TypeScript app service to Cloud Run (us-east4) that verifies GitHub webhooks and logs ping/installation/pull_request opened. No DB.

Out of Scope (for this step)
- Pub/Sub publishing, runner integration, PR comments, apply/plan logic.
- Database/persistence (unless we decide to dedupe deliveries persistently).

Architecture/Choices
- Runtime: Node.js 20, TypeScript.
- Express + `@octokit/webhooks` for signature validation.
- Endpoints: `POST /webhook`, `GET /healthz`.
- Logs: structured JSON to stdout.
- Security: HMAC signature via `GITHUB_WEBHOOK_SECRET`. Allow unauthenticated ingress.

Deliverables
- `/app` service (TS) with webhook verification + logging.
- Dockerfile + .dockerignore for `/app`.
- `app/deploy.sh` for build/push/deploy.
- GitHub App configured to send webhooks to Cloud Run URL.

Environment
- `GITHUB_WEBHOOK_SECRET`: required. HMAC for webhook validation.
- `PORT`: optional; Cloud Run provides.

Permissions
- Cloud Run service account to run the service. GitHub App: Metadata Read, Pull requests Read.

Steps
1) Local sanity (optional)
- `cd app && npm install && export GITHUB_WEBHOOK_SECRET=dev && npm run dev`

2) Deploy
- `export GITHUB_WEBHOOK_SECRET=<your-secret>`
- `./app/deploy.sh`

3) Configure GitHub App
- Set Webhook URL to `<Cloud Run URL>/webhook` and Webhook Secret to the same value.
- Permissions: Metadata (Read), Pull Requests (Read). Subscribe to Pull request + Installation events. Install on your repo/org.

4) Verify
- Save app settings → `ping` in logs. Install/add repos → `installation.*` in logs. Open PR → `pull_request.opened` in logs.

Acceptance
- `/webhook` validates signature (401 on bad, 200 on good) and logs the expected fields.
- GitHub Delivery dashboard shows 2xx; Cloud Run logs show ping/installation/PR events.
- No database required.

Notes / Next
- Next: publish to Pub/Sub topic `plan` and implement the runner.
