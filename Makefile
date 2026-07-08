.PHONY: all build test lint clean docker-up docker-down redteam terraform-init terraform-plan terraform-apply terraform-destroy

# Go services
GATEWAY = cmd/gateway
SIEM = cmd/siem_engine
CONTROL_PLANE = cmd/control_plane
PLANNER = cmd/agent/planner
EXECUTOR = cmd/agent/executor
SUMMARIZER = cmd/agent/summarizer

# Rust daemon
WATCHDOG = daemon_watchdog

# Build output
BUILD_DIR = bin

all: build

## Build all Go services and Rust watchdog
build: build-go build-rust

build-go:
	@echo "==> Building Go services..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/adm-gateway $(GATEWAY)
	go build -o $(BUILD_DIR)/adm-siem $(SIEM)
	go build -o $(BUILD_DIR)/adm-control-plane $(CONTROL_PLANE)
	go build -o $(BUILD_DIR)/adm-planner $(PLANNER)
	go build -o $(BUILD_DIR)/adm-executor $(EXECUTOR)
	go build -o $(BUILD_DIR)/adm-summarizer $(SUMMARIZER)
	@echo "==> Go services built successfully"

build-rust:
	@echo "==> Building Rust watchdog..."
	cd $(WATCHDOG) && cargo build --release
	@cp $(WATCHDOG)/target/release/adm-watchdog $(BUILD_DIR)/adm-watchdog
	@echo "==> Rust watchdog built successfully"

## Run all tests
test: test-go test-rust

test-go:
	@echo "==> Running Go tests..."
	go test -v -race -coverprofile=coverage.out ./...

test-rust:
	@echo "==> Running Rust tests..."
	cd $(WATCHDOG) && cargo test

## Lint all code
lint: lint-go lint-rust

lint-go:
	@echo "==> Running Go linter..."
	golangci-lint run ./...

lint-rust:
	@echo "==> Running Rust linter..."
	cd $(WATCHDOG) && cargo fmt --check
	cd $(WATCHDOG) && cargo clippy -- -D warnings

## Format all code
fmt: fmt-go fmt-rust

fmt-go:
	@echo "==> Formatting Go code..."
	gofmt -s -w .
	goimports -w .

fmt-rust:
	@echo "==> Formatting Rust code..."
	cd $(WATCHDOG) && cargo fmt

## Docker Compose operations
docker-up:
	@echo "==> Starting Docker Compose services..."
	docker compose -f deploy/docker-compose.yml up -d

docker-down:
	@echo "==> Stopping Docker Compose services..."
	docker compose -f deploy/docker-compose.yml down

docker-logs:
	docker compose -f docker-compose.yml logs -f

## Red team tests
redteam:
	@echo "==> Running red team tests..."
	go test -v -tags=redteam ./tests/redteam/...

redteam-build:
	@echo "==> Building red team harnesses..."
	@mkdir -p $(BUILD_DIR)
	cd tests/redteam && go build -o $(BUILD_DIR)/redteam-runner .

## Clean build artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out
	cd $(WATCHDOG) && cargo clean
	@echo "==> Clean complete"

## Development helpers
proto:
	@echo "==> Generating protobuf code..."
	protoc --go_out=. --go-grpc_out=. pkg/proto/*.proto

run-gateway: build-go
	@echo "==> Starting Gateway..."
	$(BUILD_DIR)/adm-gateway

run-siem: build-go
	@echo "==> Starting SIEM Engine..."
	$(BUILD_DIR)/adm-siem

## Help
help:
	@echo "Available targets:"
	@echo "  all          - Build everything (default)"
	@echo "  build        - Build all Go services and Rust watchdog"
	@echo "  test         - Run all tests"
	@echo "  lint         - Lint all code"
	@echo "  fmt          - Format all code"
	@echo "  docker-up    - Start Docker Compose services"
	@echo "  docker-down  - Stop Docker Compose services"
	@echo "  redteam      - Run red team tests"
	@echo "  redteam-build - Build red team harnesses"
	@echo "  clean        - Clean build artifacts"
	@echo "  proto        - Generate protobuf code"
	@echo "  run-gateway  - Build and run Gateway"
	@echo "  run-siem     - Build and run SIEM Engine"
	@echo "  terraform-init   - Initialize Terraform"
	@echo "  terraform-plan   - Plan Terraform changes"
	@echo "  terraform-apply  - Apply Terraform changes"
	@echo "  terraform-destroy - Destroy Terraform resources"
	@echo "  help         - Show this help"

## Terraform operations for Oracle Cloud Always Free
TERRAFORM_DIR = deploy/terraform

terraform-init:
	@echo "==> Initializing Terraform..."
	cd $(TERRAFORM_DIR) && terraform init

terraform-plan:
	@echo "==> Planning Terraform changes..."
	cd $(TERRAFORM_DIR) && terraform plan

terraform-apply:
	@echo "==> Applying Terraform changes..."
	cd $(TERRAFORM_DIR) && terraform apply

terraform-destroy:
	@echo "==> Destroying Terraform resources..."
	cd $(TERRAFORM_DIR) && terraform destroy

terraform-output:
	@echo "==> Getting Terraform output..."
	cd $(TERRAFORM_DIR) && terraform output
