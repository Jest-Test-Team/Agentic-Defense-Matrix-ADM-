# Multi-stage Dockerfile for Go services
# Usage: docker build --target <service> .

# ============================================
# Build stage
# ============================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build all service binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/adm-gateway ./cmd/gateway

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/adm-siem ./cmd/siem_engine

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/adm-planner ./cmd/agent/planner

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/adm-executor ./cmd/agent/executor

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/adm-summarizer ./cmd/agent/summarizer

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/adm-control-plane ./cmd/control_plane

# ============================================
# Runtime stages
# ============================================
FROM gcr.io/distroless/static-debian12 AS runtime

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Gateway
FROM runtime AS gateway
COPY --from=builder /bin/adm-gateway /usr/local/bin/adm-gateway
EXPOSE 8080 9090
ENTRYPOINT ["adm-gateway"]

# SIEM Engine
FROM runtime AS siem
COPY --from=builder /bin/adm-siem /usr/local/bin/adm-siem
EXPOSE 9091
ENTRYPOINT ["adm-siem"]

# Planner Agent
FROM runtime AS planner
COPY --from=builder /bin/adm-planner /usr/local/bin/adm-planner
EXPOSE 9081
ENTRYPOINT ["adm-planner"]

# Executor Agent
FROM runtime AS executor
COPY --from=builder /bin/adm-executor /usr/local/bin/adm-executor
EXPOSE 9082
ENTRYPOINT ["adm-executor"]

# Summarizer Agent
FROM runtime AS summarizer
COPY --from=builder /bin/adm-summarizer /usr/local/bin/adm-summarizer
EXPOSE 9083
ENTRYPOINT ["adm-summarizer"]

# Control Plane
FROM runtime AS control-plane
COPY --from=builder /bin/adm-control-plane /usr/local/bin/adm-control-plane
EXPOSE 9092
ENTRYPOINT ["adm-control-plane"]
