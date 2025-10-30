#!/usr/bin/env bash
set -euo pipefail

# Minimal deploy script. Requirements:
# - export GITHUB_WEBHOOK_SECRET=<your-secret>
# - Artifact Registry repo exists and docker is authenticated
# - Service account exists and has permissions

PROJECT="devstage-419614"
REGION="us-east4"
REPO="runners-poc"
SERVICE="app"
SA_EMAIL="app-service-sa@${PROJECT}.iam.gserviceaccount.com"

if [[ -z "${GITHUB_WEBHOOK_SECRET:-}" ]]; then
  echo "GITHUB_WEBHOOK_SECRET is not set.\nExport it first: export GITHUB_WEBHOOK_SECRET=..." >&2
  exit 1
fi
if [[ -z "${GITHUB_APP_ID:-}" ]]; then
  echo "GITHUB_APP_ID is not set.\nExport it first: export GITHUB_APP_ID=..." >&2
  exit 1
fi
if [[ -z "${GITHUB_PRIVATE_KEY_PEM:-}" ]]; then
  echo "GITHUB_PRIVATE_KEY_PEM is not set.\nExport it first: export GITHUB_PRIVATE_KEY_PEM=\"-----BEGIN PRIVATE KEY-----\\n...\"" >&2
  exit 1
fi

DIR="$(cd "$(dirname "$0")" && pwd)"
SHORT_SHA="$(git rev-parse --short HEAD)"
IMAGE="${REGION}-docker.pkg.dev/${PROJECT}/${REPO}/${SERVICE}:${SHORT_SHA}"

echo "Building ${IMAGE}"
DOCKER_DEFAULT_PLATFORM=linux/amd64 docker build -t "${IMAGE}" -f "${DIR}/Dockerfile" "${DIR}"

echo "Pushing ${IMAGE}"
docker push "${IMAGE}"

echo "Deploying to Cloud Run"
gcloud run deploy "${SERVICE}" \
  --project="${PROJECT}" \
  --region="${REGION}" \
  --image="${IMAGE}" \
  --allow-unauthenticated \
  --service-account="${SA_EMAIL}" \
  --set-env-vars="GITHUB_WEBHOOK_SECRET=${GITHUB_WEBHOOK_SECRET}" \
  --set-env-vars="GITHUB_APP_ID=${GITHUB_APP_ID}" \
  --set-env-vars="GITHUB_PRIVATE_KEY_PEM=${GITHUB_PRIVATE_KEY_PEM}"

URL="$(gcloud run services describe "${SERVICE}" --project="${PROJECT}" --region="${REGION}" --format='value(status.url)')"
echo "URL: ${URL}"
echo "Webhook: ${URL}/webhook"
