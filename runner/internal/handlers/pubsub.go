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

        plan, err := executor.CloneAndPlan(ctx, &req)
        if err != nil { plan = "tofu execution failed:\n" + err.Error() }

        if err := gh.UpsertPRComment(ctx, &req, plan); err != nil {
            logging.Log("error", map[string]any{"msg":"comment failed","error":err.Error()})
        }
        w.WriteHeader(http.StatusNoContent)
    }
}

