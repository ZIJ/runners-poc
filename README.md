# Managed Runners POC (OpenTofu)

A proof‑of‑concept for “Vercel‑for‑Terraform”, but powered by OpenTofu. It uses a GitHub App to react to pull requests, sends jobs over Google Pub/Sub, and a runner service executes `opentofu plan` and posts the result back to the PR as a comment. Both services run on Google Cloud Run in `us-east4`.

## Components
- GitHub App service (TypeScript): receives PR webhooks and logs key events (now); will enqueue jobs to Pub/Sub.
- Runner service (Go): pulls jobs from Pub/Sub, checks out the repo, runs OpenTofu, and comments the plan result on the PR.
- Google Cloud Pub/Sub: decouples webhook handling from execution via topics/subscriptions (topic: `plan`).

## Tech/Constraints
- OpenTofu (not Terraform).
- Deployed to Cloud Run in `us-east4`.
- Communicate via Pub/Sub. Env vars for POC; harden later.

## Status
- App service scaffolding done; deployable to Cloud Run; logs ping/installation/PR opened events. Runner TBD.

## Next
- Wire App → Pub/Sub (`plan`) and implement `/runner` (Go) to execute `opentofu plan` and comment back.

## Deploy (App service)
- Set `GITHUB_WEBHOOK_SECRET` and run `./app/deploy.sh`. Use the printed URL as your GitHub App webhook endpoint (`/webhook`).
