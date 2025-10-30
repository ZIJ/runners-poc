#!/usr/bin/env bash
set -euo pipefail

# Minimal deploy script for runner service

PROJECT="devstage-419614"
REGION="us-east4"
REPO="runners-poc"
SERVICE="runner"
SA_EMAIL="runner-service-sa@${PROJECT}.iam.gserviceaccount.com"

DIR="$(cd "$(dirname "$0")" && pwd)"
SHORT_SHA="$(git rev-parse --short HEAD)"
IMAGE="${REGION}-docker.pkg.dev/${PROJECT}/${REPO}/${SERVICE}:${SHORT_SHA}"

echo "Building ${IMAGE}"
DOCKER_DEFAULT_PLATFORM=linux/amd64 docker build -t "${IMAGE}" -f "${DIR}/Dockerfile" "${DIR}/.."

echo "Pushing ${IMAGE}"
docker push "${IMAGE}"

echo "Deploying to Cloud Run"
gcloud run deploy "${SERVICE}" \
  --project="${PROJECT}" \
  --region="${REGION}" \
  --image="${IMAGE}" \
  --allow-unauthenticated \
  --service-account="${SA_EMAIL}" \
  --set-env-vars="PUBSUB_SUBSCRIPTION_PLAN=plan-runner"

URL="$(gcloud run services describe "${SERVICE}" --project="${PROJECT}" --region="${REGION}" --format='value(status.url)')"
echo "URL: ${URL}"
echo "Health: ${URL}/healthz"

