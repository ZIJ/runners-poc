# Task 04 — Instrument Runner Timings and Append to PR Comment

Goal
- Capture per-step execution timings in the runner (git and OpenTofu) and surface them in the sticky PR comment for visibility and debugging.

Scope (now)
- Measure timings inside the push-based runner only.
- Steps: Pub/Sub delivery latency (publishTime → request arrival), git fetch/checkout, `tofu init`, `tofu plan`, `tofu show`, and PR comment create/update.
- Append a concise “Timings” section to the PR comment. Also log as structured JSON.

Out of scope (for now)
- App-side enqueue timing and end-to-end (GitHub webhook → App → Pub/Sub) breakdown. Optional stretch: add `queued_at` in the message for richer E2E timing.

Metrics to capture
- `queue_to_runner_ms`: difference between Pub/Sub `publishTime` and the runner’s request arrival.
- `git_fetch_ms`: duration of `git fetch --depth=1 origin <sha>`.
- `git_checkout_ms`: duration of `git checkout FETCH_HEAD`.
- `tofu_init_ms`: duration of `tofu init -input=false -no-color`.
- `tofu_plan_ms`: duration of `tofu plan -input=false -no-color -out=tfplan.bin`.
- `tofu_show_ms`: duration of `tofu show -no-color tfplan.bin`.
- `comment_list_ms`: duration of listing existing comments to find the sticky marker.
- `comment_upsert_ms`: duration of POST or PATCH to create/update the comment.
- `total_run_ms`: handler start → handler finish (end-to-end inside runner).

Comment format (appended to existing body)
- Add a small “Timings” section under the plan output, e.g.:
  - Timings (ms): queue→runner=1200, git.fetch=350, git.checkout=40, tofu.init=8000, tofu.plan=12000, tofu.show=500, comment=600, total=22000
- Keep it on one line for brevity; prefix keywords are stable for easy parsing.

Implementation steps
1) Types
- Add `types.Timings` struct with fields for each metric (int64 ms). Add helpers to record/serialize.

2) Executor timing
- In `internal/executor`, split steps to capture timings:
  - Measure `git fetch` and `git checkout` separately.
  - Measure `tofu init`, `tofu plan`, `tofu show` separately.
- Return `(plan string, timings types.Timings, err error)` from `CloneAndPlan`.

3) Handler timing
- Parse Pub/Sub `publishTime` from the push envelope and compute `queue_to_runner_ms` (use RFC3339 parsing).
- Record `total_run_ms` across the handler.
- Time GitHub comment list and upsert operations.

4) Comment builder
- Update the comment body builder to accept timings and append a single-line “Timings (ms): …” summary to the bottom of the comment body (after the plan code block).
- Ensure existing sticky marker remains unchanged so updates replace the same comment.

5) Logging
- Emit a single JSON log entry per request that includes all timings and basic identifiers: `request_id`, `repo`, `pr`, `sha`, `plan_id`.

6) Error handling
- If a step fails (e.g., no .tf files or `tofu plan` error), include the timings collected so far and post an error message to PR with the timings line appended.
- Always return 204 to ack push; errors should not trigger redelivery loops.

7) Config/flags (optional)
- Add `RUNNER_TIMINGS=1` env flag to toggle comment annotation (default: enabled). Logging always includes timings.

8) Validation
- Open a PR on a repo with .tf files and confirm the comment includes the “Timings (ms)” line.
- Compare runner logs vs. comment values.
- Induce a failure (e.g., wrong `work.dir`) and confirm error comment still appends timings for earlier steps.

Stretch (later)
- Add `queued_at` (RFC3339) to PlanRequest in the app to compute: webhook_received→enqueued and enqueued→delivered.
- Emit metrics to Cloud Monitoring (OpenTelemetry) with step spans.
- Break down provider download time vs. init core by capturing `TF_PLUGIN_CACHE_DIR` hits (advanced).

Acceptance criteria
- Each plan run posts/updates a PR comment that includes a “Timings (ms)” line with at least: queue→runner, git.fetch, git.checkout, tofu.init, tofu.plan, tofu.show, comment, total.
- Runner logs contain a JSON object per run with the same timing fields and identifiers.

Notes
- Keep the timings stable and low-noise; measure only the core commands to minimize skew.
- Use `time.Now()`/`Since()` around the exact command invocations; avoid wrapping large scopes to keep attribution precise.
