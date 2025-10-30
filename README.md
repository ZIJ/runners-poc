# Managed Runners POC (OpenTofu)

A proof‑of‑concept for “Vercel‑for‑Terraform”, but powered by OpenTofu. It uses a GitHub App to react to pull requests, sends jobs over Google Pub/Sub, and a runner service executes `opentofu plan` and posts the result back to the PR as a comment. Both services run on Google Cloud Run in `us-east4`.

## Components
- GitHub App service (TypeScript): receives PR webhooks, determines what to plan, and enqueues jobs to Pub/Sub.
- Runner service (Go): pulls jobs from Pub/Sub, checks out the repo, runs OpenTofu, and comments the plan result on the PR.
- Google Cloud Pub/Sub: decouples webhook handling from execution via topics/subscriptions.

## Tech/Constraints
- OpenTofu (not Terraform).
- Deployed to Cloud Run in `us-east4`.
- Communicate via Pub/Sub. Secrets via Secret Manager.

## Status
- POC scaffolding and docs. Implementation to follow.

## Next
- Scaffold `/app` (TS) and `/runner` (Go), wire Pub/Sub, minimal E2E: PR → plan → PR comment.
