import express from "express";
import { Webhooks, createNodeMiddleware } from "@octokit/webhooks";
import { PubSub } from "@google-cloud/pubsub";
import { createAppAuth } from "@octokit/auth-app";

const PORT = Number(process.env.PORT || 8080);
const WEBHOOK_SECRET = process.env.GITHUB_WEBHOOK_SECRET;
const PUBSUB_TOPIC = process.env.PUBSUB_TOPIC_PLAN || "plan";
const GITHUB_APP_ID = process.env.GITHUB_APP_ID;
const GITHUB_PRIVATE_KEY_PEM = process.env.GITHUB_PRIVATE_KEY_PEM;
const GITHUB_API_BASE_URL = process.env.GITHUB_API_BASE_URL || "https://api.github.com";

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

  // Try enqueueing a plan job (best-effort; logs error if misconfigured)
  try {
    await enqueuePlan({ payload, headSha, prNumber });
  } catch (e: any) {
    console.error(
      JSON.stringify({ level: "error", msg: "enqueue plan failed", error: e?.message || String(e) })
    );
  }
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
  try {
    await enqueuePlan({ payload, headSha, prNumber });
  } catch (e: any) {
    console.error(
      JSON.stringify({ level: "error", msg: "enqueue plan failed", error: e?.message || String(e) })
    );
  }
});

// Log ping to verify webhook setup
webhooks.on("ping", async ({ id }) => {
  console.log(JSON.stringify({ level: "info", event: "ping", delivery: id }));
});

// Log installation events to aid setup/verification
webhooks.on(["installation.created", "installation.deleted"], async ({ id, payload, name }) => {
  console.log(
    JSON.stringify({
      level: "info",
      event: name,
      delivery: id,
      installation_id: payload.installation?.id,
      account: payload.installation?.account?.login,
    })
  );
});
webhooks.on([
  "installation_repositories.added",
  "installation_repositories.removed",
], async ({ id, payload, name }) => {
  console.log(
    JSON.stringify({
      level: "info",
      event: name,
      delivery: id,
      installation_id: payload.installation?.id,
      repos_added: (payload.repositories_added || []).map((r: any) => r.full_name),
      repos_removed: (payload.repositories_removed || []).map((r: any) => r.full_name),
    })
  );
});

webhooks.onError((error) => {
  console.error(
    JSON.stringify({ level: "error", msg: error.message, name: error.name, stack: error.stack })
  );
});

// Mount Octokit middleware at root with explicit path matching.
// Important: do not prefix the mount path and the option path simultaneously.
app.use(createNodeMiddleware(webhooks, { path: "/webhook" }));

app.listen(PORT, () => {
  console.log(JSON.stringify({ level: "info", msg: `app listening on :${PORT}` }));
});

// Minimal publisher: mint installation token and publish to Pub/Sub topic `plan`.
async function enqueuePlan(args: { payload: any; headSha?: string; prNumber: number }) {
  const { payload, headSha, prNumber } = args;
  if (!GITHUB_APP_ID || !GITHUB_PRIVATE_KEY_PEM) {
    throw new Error("GITHUB_APP_ID and GITHUB_PRIVATE_KEY_PEM must be set to publish plan jobs");
  }
  const installationId = payload.installation?.id;
  if (!installationId) throw new Error("missing installation id on webhook payload");

  const auth = createAppAuth({ appId: Number(GITHUB_APP_ID), privateKey: GITHUB_PRIVATE_KEY_PEM });
  const { token } = await auth({ type: "installation", installationId });

  const repo = payload.repository;
  const planId = `pr-${prNumber}-${(headSha || "").slice(0, 7)}`;

  const message = {
    request_id: payload.delivery || payload?.repository?.id + ":" + String(Date.now()),
    repo: {
      full_name: `${repo.owner.login}/${repo.name}`,
      clone_url: repo.clone_url || `https://github.com/${repo.owner.login}/${repo.name}.git`,
      default_branch: repo.default_branch || "main",
    },
    pull_request: {
      number: prNumber,
      head_sha: headSha,
      head_ref: payload.pull_request?.head?.ref,
      base_ref: payload.pull_request?.base?.ref,
    },
    installation: {
      id: installationId,
      token,
    },
    work: {
      dir: ".",
      tofu_version: process.env.TOFU_VERSION || "",
      plan_id: planId,
    },
    github_api_base_url: GITHUB_API_BASE_URL,
  };

  const pubsub = new PubSub();
  const dataBuffer = Buffer.from(JSON.stringify(message));
  await pubsub.topic(PUBSUB_TOPIC).publishMessage({ data: dataBuffer });

  console.log(
    JSON.stringify({ level: "info", msg: "plan job enqueued", topic: PUBSUB_TOPIC, pr: prNumber, plan_id: planId })
  );
}
