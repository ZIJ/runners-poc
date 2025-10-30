package executor

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"

    "runner/internal/types"
)

func CloneAndPlan(ctx context.Context, req *types.PlanRequest) (string, types.Timings, error) {
    var t types.Timings
    tmp, err := os.MkdirTemp("", "runner-*")
    if err != nil { return "", t, err }
    defer os.RemoveAll(tmp)

    repoDir := filepath.Join(tmp, "repo")
    if err := os.MkdirAll(repoDir, 0o755); err != nil { return "", t, err }

    remoteURL, err := buildTokenRemoteURL(req.Repo.CloneURL, req.Repo.FullName, req.Installation.Token)
    if err != nil { return "", t, err }

    _ = runCmd(ctx, tmp, "git", "config", "--global", "--add", "safe.directory", repoDir)
    if err := runCmd(ctx, repoDir, "git", "init"); err != nil { return "", t, err }
    if err := runCmd(ctx, repoDir, "git", "remote", "add", "origin", remoteURL); err != nil { return "", t, err }
    if ms, err := timeIt(func() error { return runCmd(ctx, repoDir, "git", "fetch", "--depth=1", "origin", req.PullRequest.HeadSHA) }); err != nil {
        return "", t, fmt.Errorf("git fetch: %w", err)
    } else { t.GitFetchMS = ms }
    if ms, err := timeIt(func() error { return runCmd(ctx, repoDir, "git", "checkout", "FETCH_HEAD") }); err != nil {
        return "", t, err
    } else { t.GitCheckoutMS = ms }

    workdir := repoDir
    if strings.TrimSpace(req.Work.Dir) != "" && req.Work.Dir != "." {
        workdir = filepath.Join(repoDir, req.Work.Dir)
    }
    if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
        return "", t, fmt.Errorf("work.dir not found: %s", workdir)
    }

    if ms, err := timeIt(func() error { return runCmd(ctx, workdir, "tofu", "init", "-input=false", "-no-color") }); err != nil {
        return "", t, fmt.Errorf("tofu init: %w", err)
    } else { t.TofuInitMS = ms }
    if ms, err := timeIt(func() error { return runCmd(ctx, workdir, "tofu", "plan", "-input=false", "-no-color", "-out=tfplan.bin") }); err != nil {
        return "", t, fmt.Errorf("tofu plan: %w", err)
    } else { t.TofuPlanMS = ms }
    var out []byte
    if ms, o, err := timeItCapture(func() ([]byte, error) { return runCmdCapture(ctx, workdir, "tofu", "show", "-no-color", "tfplan.bin") }); err != nil {
        return "", t, fmt.Errorf("tofu show: %w", err)
    } else { t.TofuShowMS = ms; out = o }

    var planOut string = string(out)
    const maxLen = 200_000
    if len(planOut) > maxLen { planOut = planOut[:maxLen] + "\n... (truncated)" }
    return planOut, t, nil
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

// timeIt runs an action and returns its elapsed time in milliseconds and any error.
func timeIt(action func() error) (int64, error) {
    start := time.Now()
    err := action()
    return time.Since(start).Milliseconds(), err
}

// timeItCapture runs an action that returns output, and reports elapsed ms and returned values.
func timeItCapture(action func() ([]byte, error)) (int64, []byte, error) {
    start := time.Now()
    out, err := action()
    return time.Since(start).Milliseconds(), out, err
}
