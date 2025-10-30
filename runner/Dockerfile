# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
ARG TOFU_VERSION=1.7.0

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY runner/go.mod runner/go.sum ./
RUN go mod download
COPY runner/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/runner ./cmd/runner

FROM debian:bookworm-slim AS runtime
ARG TOFU_VERSION
ENV DEBIAN_FRONTEND=noninteractive
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl git unzip; \
    rm -rf /var/lib/apt/lists/*; \
    update-ca-certificates

# Install OpenTofu (linux_amd64)
RUN set -eux; \
    curl -fsSL -o /tmp/tofu.zip https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_linux_amd64.zip; \
    cd /tmp; unzip tofu.zip; \
    install -m 0755 tofu /usr/local/bin/tofu; \
    rm -f /tmp/tofu.zip tofu

WORKDIR /app
COPY --from=build /out/runner /usr/local/bin/runner

# Use non-root user
RUN useradd -m -u 10001 runner && chown -R runner:runner /app
USER runner
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/runner"]
