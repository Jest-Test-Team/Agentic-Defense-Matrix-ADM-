#!/bin/bash
# ADM Development Environment Setup
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "==> ADM Development Environment Setup"
echo "Project root: $PROJECT_ROOT"

# Check prerequisites
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "ERROR: $1 is not installed"
        exit 1
    fi
}

echo "==> Checking prerequisites..."
check_command go
check_command cargo
check_command docker
check_command docker compose
check_command protoc

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "Go version: $GO_VERSION"

RUST_VERSION=$(rustc --version | awk '{print $2}')
echo "Rust version: $RUST_VERSION"

DOCKER_VERSION=$(docker --version | awk '{print $3}' | sed 's/,//')
echo "Docker version: $DOCKER_VERSION"

# Install Go tools
echo "==> Installing Go tools..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest

# Install Rust tools
echo "==> Installing Rust tools..."
cd "$PROJECT_ROOT/daemon_watchdog"
cargo install --locked grpc-health-probe 2>/dev/null || true

# Generate protobuf code
echo "==> Generating protobuf code..."
cd "$PROJECT_ROOT"
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       pkg/proto/*.proto

# Download Go dependencies
echo "==> Downloading Go dependencies..."
go mod tidy

# Pull Docker images
echo "==> Pre-pulling Docker images..."
docker compose pull redis ollama 2>/dev/null || true

# Create required directories
echo "==> Creating directories..."
mkdir -p "$PROJECT_ROOT/bin"
mkdir -p "$PROJECT_ROOT/logs"
mkdir -p /var/run/adm 2>/dev/null || true

# Pull Ollama models
echo "==> Pre-pulling Ollama models..."
echo "Run 'ollama pull llama3.1:8b' to download the default model"

echo ""
echo "==> Setup complete!"
echo ""
echo "Next steps:"
echo "  1. Start infrastructure: docker compose up -d redis ollama"
echo "  2. Pull LLM model: ollama pull llama3.1:8b"
echo "  3. Build everything: make build"
echo "  4. Run tests: make test"
echo ""
