package handlers

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "io"
    "net/http"
    "time"

    gh "runner/internal/github"
    "runner/internal/executor"
    "runner/internal/logging"
    "runner/internal/types"
)

func PubSubPushHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        handlerStart := time.Now()
        body, err := io.ReadAll(r.Body)
        if err != nil { logging.Log("error", map[string]any{"msg":"read body","error":err.Error()}); w.WriteHeader(http.StatusOK); return }
        var env types.PubSubPush
        if err := json.Unmarshal(body, &env); err != nil {
            logging.Log("error", map[string]any{"msg":"decode envelope","error":err.Error()})
            w.WriteHeader(http.StatusOK)
            return
        }

        raw, err := base64.StdEncoding.DecodeString(env.Message.Data)
        if err != nil { logging.Log("error", map[string]any{"msg":"b64 decode","error":err.Error()}); w.WriteHeader(http.StatusOK); return }
        var req types.PlanRequest
        if err := json.Unmarshal(raw, &req); err != nil {
            logging.Log("error", map[string]any{"msg":"decode message","error":err.Error()})
            w.WriteHeader(http.StatusOK)
            return
        }
        if req.GitHubAPIBaseURL == "" { req.GitHubAPIBaseURL = "https://api.github.com" }

        logging.Log("info", map[string]any{"msg":"message received","request_id":req.RequestID,"repo":req.Repo.FullName,"pr":req.PullRequest.Number,"sha":req.PullRequest.HeadSHA,"plan_id":req.Work.PlanID})

        ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
        defer cancel()

        // Timings container
        var timings types.Timings
        // Compute queue -> runner latency
        if pubAt, err := time.Parse(time.RFC3339Nano, env.Message.PublishTime); err == nil {
            timings.QueueToRunnerMS = time.Since(pubAt).Milliseconds()
        } else if pubAt2, err2 := time.Parse(time.RFC3339, env.Message.PublishTime); err2 == nil {
            timings.QueueToRunnerMS = time.Since(pubAt2).Milliseconds()
        }

        // Execute plan
        plan, exTimings, err := executor.CloneAndPlan(ctx, &req)
        if err != nil { plan = "tofu execution failed:\n" + err.Error() }
        // Merge executor timings
        timings.GitFetchMS = exTimings.GitFetchMS
        timings.GitCheckoutMS = exTimings.GitCheckoutMS
        timings.TofuInitMS = exTimings.TofuInitMS
        timings.TofuPlanMS = exTimings.TofuPlanMS
        timings.TofuShowMS = exTimings.TofuShowMS

        // Phase 1: find existing and upsert initial comment without timings line
        existingID, listMS, err := gh.FindExistingComment(ctx, &req)
        if err != nil { logging.Log("error", map[string]any{"msg":"list comments failed","error":err.Error()}) }
        timings.CommentListMS = listMS
        // Build body without timings first
        body1 := gh.BuildBody(&req, plan, nil)
        commentID, upsertMS, err := gh.UpsertCommentByID(ctx, &req, existingID, body1)
        if err != nil { logging.Log("error", map[string]any{"msg":"comment upsert failed","error":err.Error()}) }
        timings.CommentUpsertMS = upsertMS

        // Compute total up to this point (before the patch below) so the posted comment includes near-complete timing.
        timings.TotalRunMS = time.Since(handlerStart).Milliseconds()

        // Phase 2: patch comment with timings
        body2 := gh.BuildBody(&req, plan, &timings)
        if _, up2, err := gh.UpsertCommentByID(ctx, &req, commentID, body2); err != nil {
            logging.Log("error", map[string]any{"msg":"comment patch with timings failed","error":err.Error()})
        } else {
            // include additional patch cost into comment upsert for observability (optional)
            timings.CommentUpsertMS += up2
        }
        // Recompute total to include the final patch for logging
        timings.TotalRunMS = time.Since(handlerStart).Milliseconds()

        // Log timings
        logging.Log("info", map[string]any{
            "msg":"timings",
            "request_id": req.RequestID,
            "repo": req.Repo.FullName,
            "pr": req.PullRequest.Number,
            "sha": req.PullRequest.HeadSHA,
            "plan_id": req.Work.PlanID,
            "queue_to_runner_ms": timings.QueueToRunnerMS,
            "git_fetch_ms": timings.GitFetchMS,
            "git_checkout_ms": timings.GitCheckoutMS,
            "tofu_init_ms": timings.TofuInitMS,
            "tofu_plan_ms": timings.TofuPlanMS,
            "tofu_show_ms": timings.TofuShowMS,
            "comment_list_ms": timings.CommentListMS,
            "comment_upsert_ms": timings.CommentUpsertMS,
            "total_run_ms": timings.TotalRunMS,
        })
        w.WriteHeader(http.StatusNoContent)
    }
}
