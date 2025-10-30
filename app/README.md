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
- PORT: optional, defaults to 8080.

## Notes
- Do not put JSON/body parsers before the webhook middleware; the Octokit middleware reads the raw request body for signature verification.
- For production, set the webhook URL in your GitHub App to `/webhook` and configure the same secret in GitHub and your runtime.
