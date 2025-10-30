# Task 05 — Optimize OpenTofu init latency (provider cache + mirror)

Goal
- Minimize `tofu init` time by using a strict, offline provider mirror only (no network fallback). Pre-install Google and Random providers into the image so `tofu init` becomes near‑noop for those providers.

Context
- Current timings example: `tofu init ≈ 4.1s`, often the dominant step due to provider downloads and checks. We can cut this by avoiding repeated network fetches and reusing binaries.

Approach (strict)
- Use only a local filesystem mirror; disable `direct {}` network fetching entirely.
- Prewarm the image with specific versions of `hashicorp/google` and `hashicorp/random` providers at build time.
- If a repo requires other providers or different versions, `tofu init` will fail fast (by design).

Design
- Provider mirror path: `/opt/tofu-providers` (read‑only baked into the image).
- CLI config at `/etc/opentofu.d/cli.tfrc` with ONLY the filesystem mirror, no `direct {}` fallback:
```
provider_installation {
  filesystem_mirror { path = "/opt/tofu-providers" }
}
```
- Prewarm in Docker build: create a transient config that pins `hashicorp/google` and `hashicorp/random` to specific versions; run `tofu providers mirror -platform=linux_amd64 /opt/tofu-providers`.
- Set `TF_CLI_CONFIG_FILE=/etc/opentofu.d/cli.tfrc` so runner uses the offline mirror.

Implementation steps
1) Dockerfile
- Add build args for versions (override as needed):
  - `ARG GOOGLE_PROVIDER_VERSION=7.9.0`
  - `ARG RANDOM_PROVIDER_VERSION=3.7.2`
- After installing OpenTofu:
  - Create `/opt/tofu-providers` and `/etc/opentofu.d`.
  - Generate a minimal `main.tf` pinning `google` and `random` to `${GOOGLE_PROVIDER_VERSION}` and `${RANDOM_PROVIDER_VERSION}`.
  - Run `tofu providers mirror -platform=linux_amd64 /opt/tofu-providers`.
  - Write `/etc/opentofu.d/cli.tfrc` with only the filesystem mirror.
  - `ENV TF_CLI_CONFIG_FILE=/etc/opentofu.d/cli.tfrc`.

2) Runner code — setup
- No code changes required for basic usage; runner inherits `TF_CLI_CONFIG_FILE` and uses the offline mirror.
- Optionally, log a clear error when `tofu init` fails due to missing providers/versions to help users align their `tofu.lock.hcl`.

3) Timings
- Expect `tofu_init_ms` to drop significantly (near‑noop) when required providers are found in the baked mirror.

4) Flags/Env
- `TF_CLI_CONFIG_FILE=/etc/opentofu.d/cli.tfrc`.
- `GOOGLE_PROVIDER_VERSION` and `RANDOM_PROVIDER_VERSION` (build args) to pin versions.

5) Safety and fallback
- Network fallback is disabled by design. If a repo requires other providers or versions, `tofu init` fails with a clear error. Document how to change pinned versions in the image.
- Don’t run `-upgrade` during init; honor `tofu.lock.hcl`.

6) Validation
- Open a PR that uses `hashicorp/google` and/or `hashicorp/random` pinned to the prewarmed versions. Expect `tofu init` to be ≪ 1s.
- Open a PR requiring other providers/versions to verify fast‑fail behavior (document remediation).

7) Documentation
- Update README/AGENTS to document env vars and the mirror/cache behavior.
- Note that Cloud Run instance restarts can cold-start the cache; prewarm reduces impact but cannot eliminate it entirely.

Stretch (later)
- Support additional prewarmed providers as needed (pin versions and mirror at build time).
- Multi-platform mirrors via multiple `-platform` flags if needed.

Acceptance criteria
- With provider mirror+cache in place, `tofu init` time is materially reduced after the first warm on an instance.
- Comment timings reflect the improvement; documentation updated to explain the mechanism and envs.
