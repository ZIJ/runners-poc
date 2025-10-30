import express from "express";
import { Webhooks, createNodeMiddleware } from "@octokit/webhooks";

const PORT = Number(process.env.PORT || 8080);
const WEBHOOK_SECRET = process.env.GITHUB_WEBHOOK_SECRET;

if (!WEBHOOK_SECRET) {
  // Fail fast so misconfigured environments are obvious.
  console.error("GITHUB_WEBHOOK_SECRET is required");
  process.exit(1);
}

// Express app
const app = express();
app.disable("x-powered-by");

// Health endpoint for sanity checks
app.get("/healthz", (_req, res) => {
  res.status(200).send("ok");
});

// Webhooks setup
const webhooks = new Webhooks({ secret: WEBHOOK_SECRET });

// Log PR opened events (POC requirement)
webhooks.on("pull_request.opened", async ({ id, payload }) => {
  const repo = `${payload.repository.owner.login}/${payload.repository.name}`;
  const prNumber = payload.pull_request.number;
  const headSha = payload.pull_request.head?.sha;
  const log = {
    level: "info",
    event: "pull_request",
    action: "opened",
    delivery: id,
    repo,
    pr_number: prNumber,
    head_sha: headSha,
  };
  console.log(JSON.stringify(log));
});

// Optional: observe other PR lifecycle events for future use
webhooks.on(["pull_request.synchronize", "pull_request.reopened"], async ({ id, payload, name }) => {
  const repo = `${payload.repository.owner.login}/${payload.repository.name}`;
  const prNumber = payload.pull_request.number;
  const headSha = payload.pull_request.head?.sha;
  console.log(
    JSON.stringify({
      level: "info",
      event: name,
      delivery: id,
      repo,
      pr_number: prNumber,
      head_sha: headSha,
    })
  );
});

webhooks.onError((error) => {
  console.error(
    JSON.stringify({ level: "error", msg: error.message, name: error.name, stack: error.stack })
  );
});

// Mount Octokit middleware. Do not attach JSON/body parsers before this.
app.use("/webhook", createNodeMiddleware(webhooks, { path: "/webhook" }));

app.listen(PORT, () => {
  console.log(JSON.stringify({ level: "info", msg: `app listening on :${PORT}` }));
});

