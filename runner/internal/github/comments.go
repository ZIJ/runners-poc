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

func UpsertPRComment(ctx context.Context, req *types.PlanRequest, content string) error {
    client := &http.Client{ Timeout: 30 * time.Second }
    base := strings.TrimRight(req.GitHubAPIBaseURL, "/")
    ownerRepo := strings.Split(req.Repo.FullName, "/")
    if len(ownerRepo) != 2 { return fmt.Errorf("invalid repo.full_name") }
    owner, repo := ownerRepo[0], ownerRepo[1]

    marker := fmt.Sprintf("<!-- runners-poc:plan:%s -->", safe(req.Work.PlanID))
    body := buildBody(req, content, marker)

    listURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100", base, owner, repo, req.PullRequest.Number)
    var existingID int64
    {
        r, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
        r.Header.Set("Accept", "application/vnd.github+json")
        r.Header.Set("Authorization", "token "+req.Installation.Token)
        resp, err := client.Do(r)
        if err != nil { return err }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return fmt.Errorf("list comments: %s: %s", resp.Status, strings.TrimSpace(string(b))) }
        var comments []struct{ ID int64 `json:"id"`; Body string `json:"body"` }
        if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil { return err }
        for _, c := range comments { if strings.Contains(c.Body, marker) { existingID = c.ID; break } }
    }

    if existingID != 0 {
        patchURL := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", base, owner, repo, existingID)
        payload := map[string]string{"body": body}
        buf, _ := json.Marshal(payload)
        r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(buf))
        r.Header.Set("Accept", "application/vnd.github+json")
        r.Header.Set("Authorization", "token "+req.Installation.Token)
        r.Header.Set("Content-Type", "application/json")
        resp, err := client.Do(r)
        if err != nil { return err }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return fmt.Errorf("update comment: %s: %s", resp.Status, strings.TrimSpace(string(b))) }
        return nil
    }

    postURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", base, owner, repo, req.PullRequest.Number)
    payload := map[string]string{"body": body}
    buf, _ := json.Marshal(payload)
    r, _ := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(buf))
    r.Header.Set("Accept", "application/vnd.github+json")
    r.Header.Set("Authorization", "token "+req.Installation.Token)
    r.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(r)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return fmt.Errorf("create comment: %s: %s", resp.Status, strings.TrimSpace(string(b))) }
    return nil
}

func buildBody(req *types.PlanRequest, plan string, marker string) string {
    var b strings.Builder
    fmt.Fprintf(&b, "%s\n", marker)
    fmt.Fprintf(&b, "Managed Runners (OpenTofu) Plan\n\n")
    fmt.Fprintf(&b, "%s @ %s\n\n", req.Repo.FullName, req.PullRequest.HeadSHA)
    if strings.TrimSpace(req.Work.Dir) != "" && req.Work.Dir != "." { fmt.Fprintf(&b, "Dir: %s\n\n", req.Work.Dir) }
    b.WriteString("```\n")
    b.WriteString(plan)
    if !strings.HasSuffix(plan, "\n") { b.WriteString("\n") }
    b.WriteString("```\n")
    return b.String()
}

func safe(s string) string { if s == "" { return "default" }; return s }

