# Task 03 — Runner Service (Push delivery over HTTP)

Goal
- Convert the runner to a push-based Cloud Run HTTP service that accepts Pub/Sub push requests, executes OpenTofu plan, and posts a sticky PR comment.

Summary
- Endpoint `POST /pubsub/push` receives Pub/Sub’s JSON envelope, base64-decodes the message `data`, parses `PlanRequest`, clones the repo at PR head SHA, runs `tofu init/plan/show` with `--no-color`, and upserts a single PR comment keyed by `<!-- runners-poc:plan:{plan_id} -->`. Returns `204` to acknowledge delivery.

Code layout
- `runner/cmd/runner/main.go`: HTTP server bootstrap using `chi` with middleware and routes.
- `runner/internal/handlers/pubsub.go`: Pub/Sub push handler (decode envelope, execute, comment).
- `runner/internal/executor/executor.go`: Git clone + OpenTofu execution.
- `runner/internal/github/comments.go`: Create/update sticky PR comment via REST.
- `runner/internal/types/types.go`: `PlanRequest` schema and Pub/Sub push envelope.
- `runner/internal/logging/logging.go`: tiny JSON logger utility.

Message schema (PlanRequest)
- Same as Task 02. Required: `repo.full_name`, `repo.clone_url`, `pull_request.number`, `pull_request.head_sha`, `installation.token`, `work.plan_id`. Optional: `work.dir` (default `.`), `github_api_base_url` (default `https://api.github.com`).

Deployment (Runner)
1) Build and deploy the runner image to Cloud Run (us-east4). The provided `runner/deploy.sh` builds the binary and deploys; for production, prefer authenticated access (see below).
2) Create a Pub/Sub push subscription pointing at the runner endpoint:
   - Topic: `plan`
   - Subscription: `plan-runner` (push)
   - Push endpoint: `https://<runner-url>/pubsub/push`
   - Push auth: OIDC with a dedicated service account that has `roles/run.invoker` on the runner.

Example commands
```
PROJECT=devstage-419614
REGION=us-east4
RUNNER_URL="$(gcloud run services describe runner --project=$PROJECT --region=$REGION --format='value(status.url)')"

# Create a dedicated push service account (once)
gcloud iam service-accounts create pubsub-push-sa --project $PROJECT \
  --display-name "Pub/Sub Push to Runner"

# Allow it to invoke the Cloud Run service
gcloud run services add-iam-policy-binding runner \
  --project $PROJECT --region $REGION \
  --member serviceAccount:pubsub-push-sa@$PROJECT.iam.gserviceaccount.com \
  --role roles/run.invoker

# Create push subscription to /pubsub/push
gcloud pubsub subscriptions create plan-runner \
  --project $PROJECT \
  --topic plan \
  --push-endpoint="${RUNNER_URL}/pubsub/push" \
  --push-auth-service-account=pubsub-push-sa@$PROJECT.iam.gserviceaccount.com \
  --push-auth-token-audience="${RUNNER_URL}/pubsub/push"
```

IAM
- App service account: needs `roles/pubsub.publisher` on topic `plan`.
- Runner Cloud Run service account: no Pub/Sub role needed for push delivery; does need Artifact Registry pull.
- Pub/Sub push service account: needs `roles/run.invoker` on runner.

Operational notes
- Runner returns `204` on all deliveries; errors are written to PR as a comment and logged to stdout. This avoids retry loops for non-retriable module issues (e.g., missing .tf files).
- Prefer `--no-color` for readable comments. Large outputs are truncated (~200 KB).
- If the module is not at repo root, set `work.dir` in the message.

Validate
1) Open a PR in a repo with .tf files (at `.` or the path you set in `work.dir`).
2) Confirm the App logs `plan job enqueued`.
3) Confirm runner logs show `message received` and a PR comment appears/updates.

Next
- Add optional OIDC token verification in the handler (validate audience/issuer).
- Provider warm-start and repo checkout caching.
- Coalescing plan runs per PR head.

