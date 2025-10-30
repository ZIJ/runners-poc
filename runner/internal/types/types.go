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

