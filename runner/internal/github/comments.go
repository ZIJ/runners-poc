package github

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "runner/internal/types"
)

func BuildBody(req *types.PlanRequest, plan string, timings *types.Timings) string {
    marker := fmt.Sprintf("<!-- runners-poc:plan:%s -->", safe(req.Work.PlanID))
    var b strings.Builder
    fmt.Fprintf(&b, "%s\n", marker)
    fmt.Fprintf(&b, "Managed Runners (OpenTofu) Plan\n\n")
    fmt.Fprintf(&b, "%s @ %s\n\n", req.Repo.FullName, req.PullRequest.HeadSHA)
    if strings.TrimSpace(req.Work.Dir) != "" && req.Work.Dir != "." { fmt.Fprintf(&b, "Dir: %s\n\n", req.Work.Dir) }
    b.WriteString("```\n")
    b.WriteString(plan)
    if !strings.HasSuffix(plan, "\n") { b.WriteString("\n") }
    b.WriteString("```\n")
    // Append timings line if provided
    if timings != nil {
        fmt.Fprintf(&b, "Timings (ms): queue→runner=%d, git.fetch=%d, git.checkout=%d, tofu.init=%d, tofu.plan=%d, tofu.show=%d, comment.list=%d, comment.upsert=%d, total=%d\n",
            timings.QueueToRunnerMS, timings.GitFetchMS, timings.GitCheckoutMS, timings.TofuInitMS, timings.TofuPlanMS, timings.TofuShowMS, timings.CommentListMS, timings.CommentUpsertMS, timings.TotalRunMS,
        )
    }
    return b.String()
}

// FindExistingComment lists PR comments and returns the id of the sticky comment (if any) and the list duration in ms.
func FindExistingComment(ctx context.Context, req *types.PlanRequest) (int64, int64, error) {
    client := &http.Client{ Timeout: 30 * time.Second }
    base := strings.TrimRight(req.GitHubAPIBaseURL, "/")
    ownerRepo := strings.Split(req.Repo.FullName, "/")
    if len(ownerRepo) != 2 { return 0, 0, fmt.Errorf("invalid repo.full_name") }
    owner, repo := ownerRepo[0], ownerRepo[1]
    marker := fmt.Sprintf("<!-- runners-poc:plan:%s -->", safe(req.Work.PlanID))

    listURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100", base, owner, repo, req.PullRequest.Number)
    start := time.Now()
    r, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
    r.Header.Set("Accept", "application/vnd.github+json")
    r.Header.Set("Authorization", "token "+req.Installation.Token)
    resp, err := client.Do(r)
    if err != nil { return 0, 0, err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return 0, 0, fmt.Errorf("list comments: %s: %s", resp.Status, strings.TrimSpace(string(b))) }
    var comments []struct{ ID int64 `json:"id"`; Body string `json:"body"` }
    if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil { return 0, 0, err }
    var existingID int64
    for _, c := range comments { if strings.Contains(c.Body, marker) { existingID = c.ID; break } }
    return existingID, time.Since(start).Milliseconds(), nil
}

// UpsertCommentByID creates or updates a comment. If existingID==0, creates; otherwise patches. Returns the comment id and upsert duration in ms.
func UpsertCommentByID(ctx context.Context, req *types.PlanRequest, existingID int64, body string) (int64, int64, error) {
    client := &http.Client{ Timeout: 30 * time.Second }
    base := strings.TrimRight(req.GitHubAPIBaseURL, "/")
    ownerRepo := strings.Split(req.Repo.FullName, "/")
    if len(ownerRepo) != 2 { return 0, 0, fmt.Errorf("invalid repo.full_name") }
    owner, repo := ownerRepo[0], ownerRepo[1]
    start := time.Now()
    if existingID != 0 {
        patchURL := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", base, owner, repo, existingID)
        payload := map[string]string{"body": body}
        buf, _ := json.Marshal(payload)
        r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(buf))
        r.Header.Set("Accept", "application/vnd.github+json")
        r.Header.Set("Authorization", "token "+req.Installation.Token)
        r.Header.Set("Content-Type", "application/json")
        resp, err := client.Do(r)
        if err != nil { return 0, 0, err }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return 0, 0, fmt.Errorf("update comment: %s: %s", resp.Status, strings.TrimSpace(string(b))) }
        return existingID, time.Since(start).Milliseconds(), nil
    }
    postURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", base, owner, repo, req.PullRequest.Number)
    payload := map[string]string{"body": body}
    buf, _ := json.Marshal(payload)
    r, _ := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(buf))
    r.Header.Set("Accept", "application/vnd.github+json")
    r.Header.Set("Authorization", "token "+req.Installation.Token)
    r.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(r)
    if err != nil { return 0, 0, err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return 0, 0, fmt.Errorf("create comment: %s: %s", resp.Status, strings.TrimSpace(string(b))) }
    // Decode id from response
    var res struct { ID int64 `json:"id"` }
    if err := json.NewDecoder(resp.Body).Decode(&res); err != nil { return 0, 0, err }
    return res.ID, time.Since(start).Milliseconds(), nil
}

func safe(s string) string { if s == "" { return "default" }; return s }
