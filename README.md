# Managed Runners POC (OpenTofu)

A proof‑of‑concept for “Vercel‑for‑Terraform”, but powered by OpenTofu. It uses a GitHub App to react to pull requests, sends jobs over Google Pub/Sub, and a runner service executes `opentofu plan` and posts the result back to the PR as a comment. Both services run on Google Cloud Run in `us-east4`.

## Components
- GitHub App service (TypeScript): receives PR webhooks, logs key events, and enqueues jobs to Pub/Sub topic `plan`.
- Runner service (Go): HTTP service on Cloud Run. Receives Pub/Sub push deliveries at `/pubsub/push`, checks out the repo, runs OpenTofu, and posts/updates a sticky PR comment.
- Google Cloud Pub/Sub: decouples webhook handling from execution via topic `plan` and a push subscription `plan-runner`.

## Tech/Constraints
- OpenTofu (not Terraform).
- Deployed to Cloud Run in `us-east4`.
- Communicate via Pub/Sub. Env vars for POC; harden later.

## Status
- App service publishes `PlanRequest` messages to `plan` on PR events.
- Runner service (push) executes plans and comments results.

## Deploy (App service)
- Set `GITHUB_WEBHOOK_SECRET`, `GITHUB_APP_ID`, `GITHUB_PRIVATE_KEY_PEM` and run `./app/deploy.sh`. Use the printed URL as your GitHub App webhook endpoint (`/webhook`).

## Deploy (Runner service)
- Build and deploy with `./runner/deploy.sh`.
- Create a Pub/Sub push subscription to the runner URL:
  - `plan-runner` → `https://<runner-url>/pubsub/push`
  - Use OIDC push auth with a service account that has `roles/run.invoker` on the runner.

See `agent-tasks/03-runner-service-push-delivery.md` for detailed wiring and IAM steps.
