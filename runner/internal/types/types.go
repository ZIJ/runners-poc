package types

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
        Dir     string `json:"dir"`
        TofuVer string `json:"tofu_version"`
        PlanID  string `json:"plan_id"`
    } `json:"work"`
    GitHubAPIBaseURL string `json:"github_api_base_url"`
}

// Pub/Sub push request body
type PubSubPush struct {
    Message struct {
        Attributes  map[string]string `json:"attributes"`
        Data        string            `json:"data"`
        MessageID   string            `json:"messageId"`
        PublishTime string            `json:"publishTime"`
    } `json:"message"`
    Subscription string `json:"subscription"`
}

// Timings holds per-step durations in milliseconds.
type Timings struct {
    QueueToRunnerMS int64 `json:"queue_to_runner_ms"`
    GitFetchMS      int64 `json:"git_fetch_ms"`
    GitCheckoutMS   int64 `json:"git_checkout_ms"`
    TofuInitMS      int64 `json:"tofu_init_ms"`
    TofuPlanMS      int64 `json:"tofu_plan_ms"`
    TofuShowMS      int64 `json:"tofu_show_ms"`
    CommentListMS   int64 `json:"comment_list_ms"`
    CommentUpsertMS int64 `json:"comment_upsert_ms"`
    TotalRunMS      int64 `json:"total_run_ms"`
}
