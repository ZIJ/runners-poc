# App Service (GitHub Webhooks)

Minimal TypeScript/Express service that verifies GitHub App webhooks and logs pull_request opened events. Intended to run on Cloud Run (us-east4), but can run locally for development.

## Quickstart (Local)

Prereqs
- Node.js 20+
- npm

Steps
- cd app
- npm install
- export GITHUB_WEBHOOK_SECRET=devsecret
- npm run dev

Health check
- curl http://localhost:8080/healthz → ok

## Environment
- GITHUB_WEBHOOK_SECRET: required. HMAC secret to verify signatures.
- GITHUB_APP_ID: required for end-to-end (publishing plan jobs).
- GITHUB_PRIVATE_KEY_PEM: required for end-to-end (mint installation tokens).
- PUBSUB_TOPIC_PLAN: optional, defaults to `plan`.
- PORT: optional, defaults to 8080.

## Notes
- Do not put JSON/body parsers before the webhook middleware; the Octokit middleware reads the raw request body for signature verification.
- For production, set the webhook URL in your GitHub App to `/webhook` and configure the same secret in GitHub and your runtime.
- To test end-to-end with the runner, add GitHub App permissions: Issues (Read & write) and Contents (Read). Ensure a Pub/Sub topic `plan` exists and the app service account has `roles/pubsub.publisher` on it.

## Deploy
- Export envs: `GITHUB_WEBHOOK_SECRET`, `GITHUB_APP_ID`, `GITHUB_PRIVATE_KEY_PEM`.
- Run: `./app/deploy.sh`.
- Use the printed URL as your GitHub App webhook endpoint (`/webhook`).
