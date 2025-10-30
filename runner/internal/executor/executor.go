package executor

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "runner/internal/types"
)

func CloneAndPlan(ctx context.Context, req *types.PlanRequest) (string, error) {
    tmp, err := os.MkdirTemp("", "runner-*")
    if err != nil { return "", err }
    defer os.RemoveAll(tmp)

    repoDir := filepath.Join(tmp, "repo")
    if err := os.MkdirAll(repoDir, 0o755); err != nil { return "", err }

    remoteURL, err := buildTokenRemoteURL(req.Repo.CloneURL, req.Repo.FullName, req.Installation.Token)
    if err != nil { return "", err }

    _ = runCmd(ctx, tmp, "git", "config", "--global", "--add", "safe.directory", repoDir)
    if err := runCmd(ctx, repoDir, "git", "init"); err != nil { return "", err }
    if err := runCmd(ctx, repoDir, "git", "remote", "add", "origin", remoteURL); err != nil { return "", err }
    if err := runCmd(ctx, repoDir, "git", "fetch", "--depth=1", "origin", req.PullRequest.HeadSHA); err != nil { return "", fmt.Errorf("git fetch: %w", err) }
    if err := runCmd(ctx, repoDir, "git", "checkout", "FETCH_HEAD"); err != nil { return "", err }

    workdir := repoDir
    if strings.TrimSpace(req.Work.Dir) != "" && req.Work.Dir != "." {
        workdir = filepath.Join(repoDir, req.Work.Dir)
    }
    if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
        return "", fmt.Errorf("work.dir not found: %s", workdir)
    }

    if err := runCmd(ctx, workdir, "tofu", "init", "-input=false", "-no-color"); err != nil { return "", fmt.Errorf("tofu init: %w", err) }
    if err := runCmd(ctx, workdir, "tofu", "plan", "-input=false", "-no-color", "-out=tfplan.bin"); err != nil { return "", fmt.Errorf("tofu plan: %w", err) }
    out, err := runCmdCapture(ctx, workdir, "tofu", "show", "-no-color", "tfplan.bin")
    if err != nil { return "", fmt.Errorf("tofu show: %w", err) }

    var planOut string = string(out)
    const maxLen = 200_000
    if len(planOut) > maxLen { planOut = planOut[:maxLen] + "\n... (truncated)" }
    return planOut, nil
}

func buildTokenRemoteURL(cloneURL, fullName, token string) (string, error) {
    host := "github.com"
    if cloneURL != "" {
        if u, err := url.Parse(cloneURL); err == nil && u.Host != "" { host = u.Host }
    }
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

// Small helper for JSON pretty error snippets when needed.
func toJSON(v any) string {
    b, _ := json.Marshal(v)
    return string(b)
}

