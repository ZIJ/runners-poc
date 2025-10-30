package main

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"

    "cloud.google.com/go/pubsub"
)

type PlanRequest struct {
    RequestID string `json:"request_id"`
    Repo struct {
        FullName   string `json:"full_name"`
        CloneURL   string `json:"clone_url"`
        DefaultRef string `json:"default_branch"`
    } `json:"repo"`
    PullRequest struct {
        Number  int    `json:"number"`
        HeadSHA string `json:"head_sha"`
        HeadRef string `json:"head_ref"`
        BaseRef string `json:"base_ref"`
    } `json:"pull_request"`
    Installation struct {
        ID    int64  `json:"id"`
        Token string `json:"token"`
    } `json:"installation"`
    Work struct {
        Dir       string `json:"dir"`
        TofuVer   string `json:"tofu_version"`
        PlanID    string `json:"plan_id"`
    } `json:"work"`
    GitHubAPIBaseURL string `json:"github_api_base_url"`
}

func getenv(key, def string) string {
    v := os.Getenv(key)
    if v == "" {
        return def
    }
    return v
}

func main() {
    ctx := context.Background()

    // Minimal health server
    go func() {
        mux := http.NewServeMux()
        mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
        port := getenv("PORT", "8080")
        _ = http.ListenAndServe(":"+port, mux)
    }()

    projectID := detectProjectID(ctx)
    if projectID == "" {
        logJSON("error", map[string]any{"msg": "project id not found (set GOOGLE_CLOUD_PROJECT or enable metadata)"})
        os.Exit(1)
    }

    subName := getenv("PUBSUB_SUBSCRIPTION_PLAN", "plan-runner")

    client, err := pubsub.NewClient(ctx, projectID)
    if err != nil {
        logJSON("error", map[string]any{"msg": "pubsub client", "error": err.Error()})
        os.Exit(1)
    }
    defer client.Close()

    sub := client.Subscription(subName)
    sub.ReceiveSettings.Synchronous = true
    sub.ReceiveSettings.MaxOutstandingMessages = 1

    logJSON("info", map[string]any{"msg": "runner started", "project": projectID, "subscription": subName})

    err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
        start := time.Now()
        var req PlanRequest
        if err := json.Unmarshal(m.Data, &req); err != nil {
            logJSON("error", map[string]any{"msg": "decode message", "error": err.Error()})
            m.Ack()
            return
        }
        // Helpful log for visibility that a message was received
        logJSON("info", map[string]any{
            "msg":        "message received",
            "request_id": req.RequestID,
            "repo":       req.Repo.FullName,
            "pr":         req.PullRequest.Number,
            "sha":        req.PullRequest.HeadSHA,
            "plan_id":    req.Work.PlanID,
        })
        // Default API base
        if req.GitHubAPIBaseURL == "" {
            req.GitHubAPIBaseURL = "https://api.github.com"
        }
        // Basic validation
        if req.Installation.Token == "" || req.Repo.FullName == "" || req.PullRequest.Number == 0 || req.PullRequest.HeadSHA == "" {
            logJSON("error", map[string]any{"msg": "invalid message", "request_id": req.RequestID})
            m.Ack()
            return
        }

        ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
        defer cancel()
        if err := handleMessage(ctx, &req); err != nil {
            logJSON("error", map[string]any{"msg": "handle message failed", "request_id": req.RequestID, "error": err.Error()})
            m.Nack()
            return
        }
        logJSON("info", map[string]any{"msg": "completed", "request_id": req.RequestID, "duration_ms": time.Since(start).Milliseconds()})
        m.Ack()
    })

    if err != nil {
        logJSON("error", map[string]any{"msg": "subscriber error", "error": err.Error()})
        os.Exit(1)
    }
}

func handleMessage(ctx context.Context, req *PlanRequest) error {
    tmp, err := os.MkdirTemp("", "runner-*")
    if err != nil { return err }
    defer os.RemoveAll(tmp)

    repoDir := filepath.Join(tmp, "repo")
    if err := os.MkdirAll(repoDir, 0o755); err != nil { return err }

    ownerRepo := req.Repo.FullName
    if ownerRepo == "" {
        return errors.New("missing repo.full_name")
    }

    // Prepare token-authenticated remote URL
    remoteURL, err := buildTokenRemoteURL(req.Repo.CloneURL, ownerRepo, req.Installation.Token)
    if err != nil { return err }

    // Safe directory
    _ = runCmd(ctx, tmp, "git", "config", "--global", "--add", "safe.directory", repoDir)

    // Init + fetch specific SHA (shallow)
    if err := runCmd(ctx, repoDir, "git", "init"); err != nil { return err }
    if err := runCmd(ctx, repoDir, "git", "remote", "add", "origin", remoteURL); err != nil { return err }
    if err := runCmd(ctx, repoDir, "git", "fetch", "--depth=1", "origin", req.PullRequest.HeadSHA); err != nil { return fmt.Errorf("git fetch: %w", err) }
    if err := runCmd(ctx, repoDir, "git", "checkout", "FETCH_HEAD"); err != nil { return err }

    // Determine work dir
    workdir := repoDir
    if strings.TrimSpace(req.Work.Dir) != "" && req.Work.Dir != "." {
        workdir = filepath.Join(repoDir, req.Work.Dir)
    }
    if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
        return fmt.Errorf("work.dir not found: %s", workdir)
    }

    // Run OpenTofu
    if err := runCmd(ctx, workdir, "tofu", "init", "-input=false", "-no-color"); err != nil { return fmt.Errorf("tofu init: %w", err) }
    if err := runCmd(ctx, workdir, "tofu", "plan", "-input=false", "-no-color", "-out=tfplan.bin"); err != nil { return fmt.Errorf("tofu plan: %w", err) }
    out, err := runCmdCapture(ctx, workdir, "tofu", "show", "-no-color", "tfplan.bin")
    if err != nil { return fmt.Errorf("tofu show: %w", err) }
    planOut := string(out)
    const maxLen = 200_000
    if len(planOut) > maxLen { planOut = planOut[:maxLen] + "\n... (truncated)" }

    // Post or update PR comment
    return upsertPRComment(ctx, req, planOut)
}

func buildTokenRemoteURL(cloneURL, fullName, token string) (string, error) {
    host := "github.com"
    if cloneURL != "" {
        if u, err := url.Parse(cloneURL); err == nil && u.Host != "" {
            host = u.Host
        }
    }
    // put token in userinfo per GitHub guidance
    // x-access-token:<token>@host/owner/repo.git
    // token can contain chars; URL-escape it
    return fmt.Sprintf("https://x-access-token:%s@%s/%s.git", url.QueryEscape(token), host, fullName), nil
}

func runCmd(ctx context.Context, dir, name string, args ...string) error {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Dir = dir
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    cmd.Stdout = io.Discard
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("%s %v: %v | %s", name, args, err, strings.TrimSpace(stderr.String()))
    }
    return nil
}

func runCmdCapture(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Dir = dir
    var out bytes.Buffer
    var errb bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &errb
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("%s %v: %v | %s", name, args, err, strings.TrimSpace(errb.String()))
    }
    return out.Bytes(), nil
}

func upsertPRComment(ctx context.Context, req *PlanRequest, plan string) error {
    client := &http.Client{ Timeout: 30 * time.Second }
    base := strings.TrimRight(req.GitHubAPIBaseURL, "/")
    ownerRepo := strings.Split(req.Repo.FullName, "/")
    if len(ownerRepo) != 2 { return errors.New("invalid repo.full_name") }
    owner, repo := ownerRepo[0], ownerRepo[1]

    marker := fmt.Sprintf("<!-- runners-poc:plan:%s -->", safePlanID(req.Work.PlanID))
    body := buildCommentBody(req, plan, marker)

    // List recent comments (first 100)
    listURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100", base, owner, repo, req.PullRequest.Number)
    existingID := int64(0)
    {
        httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
        httpReq.Header.Set("Accept", "application/vnd.github+json")
        httpReq.Header.Set("Authorization", "token "+req.Installation.Token)
        resp, err := client.Do(httpReq)
        if err != nil { return err }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 {
            b, _ := io.ReadAll(resp.Body)
            return fmt.Errorf("list comments: %s: %s", resp.Status, strings.TrimSpace(string(b)))
        }
        var comments []struct {
            ID   int64  `json:"id"`
            Body string `json:"body"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil { return err }
        for _, c := range comments {
            if strings.Contains(c.Body, marker) { existingID = c.ID; break }
        }
    }

    if existingID != 0 {
        // Update
        patchURL := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", base, owner, repo, existingID)
        payload := map[string]string{"body": body}
        buf, _ := json.Marshal(payload)
        httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(buf))
        httpReq.Header.Set("Accept", "application/vnd.github+json")
        httpReq.Header.Set("Authorization", "token "+req.Installation.Token)
        httpReq.Header.Set("Content-Type", "application/json")
        resp, err := client.Do(httpReq)
        if err != nil { return err }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 {
            b, _ := io.ReadAll(resp.Body)
            return fmt.Errorf("update comment: %s: %s", resp.Status, strings.TrimSpace(string(b)))
        }
        logJSON("info", map[string]any{"msg": "comment updated", "comment_id": existingID})
        return nil
    }

    // Create new comment
    postURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", base, owner, repo, req.PullRequest.Number)
    payload := map[string]string{"body": body}
    buf, _ := json.Marshal(payload)
    httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(buf))
    httpReq.Header.Set("Accept", "application/vnd.github+json")
    httpReq.Header.Set("Authorization", "token "+req.Installation.Token)
    httpReq.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(httpReq)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("create comment: %s: %s", resp.Status, strings.TrimSpace(string(b)))
    }
    logJSON("info", map[string]any{"msg": "comment created"})
    return nil
}

func buildCommentBody(req *PlanRequest, plan, marker string) string {
    var b strings.Builder
    fmt.Fprintf(&b, "%s\n", marker)
    fmt.Fprintf(&b, "OpenTofu plan for %s @ %s\n\n", req.Repo.FullName, req.PullRequest.HeadSHA)
    if strings.TrimSpace(req.Work.Dir) != "" && req.Work.Dir != "." {
        fmt.Fprintf(&b, "Dir: %s\n\n", req.Work.Dir)
    }
    b.WriteString("```\n")
    b.WriteString(plan)
    if !strings.HasSuffix(plan, "\n") { b.WriteString("\n") }
    b.WriteString("```\n")
    return b.String()
}

func safePlanID(s string) string {
    if s == "" { return "default" }
    return s
}

func logJSON(level string, fields map[string]any) {
    if fields == nil { fields = map[string]any{} }
    fields["level"] = level
    enc, _ := json.Marshal(fields)
    fmt.Println(string(enc))
}

// detectProjectID returns a GCP project id from env or metadata server.
func detectProjectID(ctx context.Context) string {
    // Common env vars across GCP runtimes/tools
    for _, k := range []string{"GOOGLE_CLOUD_PROJECT", "GCP_PROJECT_ID", "GCP_PROJECT", "PROJECT_ID", "GCLOUD_PROJECT"} {
        if v := os.Getenv(k); v != "" { return v }
    }
    // Try metadata server (available in Cloud Run)
    mdURL := "http://metadata.google.internal/computeMetadata/v1/project/project-id"
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, mdURL, nil)
    if err != nil { return "" }
    req.Header.Set("Metadata-Flavor", "Google")
    client := &http.Client{ Timeout: 2 * time.Second }
    resp, err := client.Do(req)
    if err != nil { return "" }
    defer resp.Body.Close()
    if resp.StatusCode != 200 { return "" }
    b, _ := io.ReadAll(resp.Body)
    return strings.TrimSpace(string(b))
}
