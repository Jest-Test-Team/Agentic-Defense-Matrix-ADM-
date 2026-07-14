# Using the ADM Deployment on Oracle Cloud

This guide covers connecting to, operating, and troubleshooting the Agentic
Defense Matrix (ADM) instance deployed by `deploy/terraform` via the
**Terraform OCI** GitHub Actions workflow.

## Deployment at a glance

| Item | Value |
|---|---|
| Instance | `adm-instance`, compartment root, region `ap-tokyo-1` |
| Public IP | `155.248.184.176` (changes if the instance is recreated — see below) |
| Shape | `VM.Standard.E2.1.Micro` (1 OCPU / 1 GB) — auto-upgrades to `A1.Flex` 2 OCPU / 12 GB when Tokyo has ARM capacity |
| OS | Oracle Linux 8 |
| Open ports | 22 (SSH), 8080 (Gateway API), 11434 (Ollama) — enforced by the `adm-nsg` network security group |
| Data volume | `adm-data`, 50 GB, paravirtualized attachment |

To get the current public IP after any re-apply, check the **Terraform
output** step of the latest apply run in GitHub Actions, or the OCI console
(Compute → Instances → adm-instance).

## 1. Connect via SSH

The instance trusts the public key stored in the `ADM_SSH_PUBLIC_KEY` GitHub
secret. Use its private counterpart. On Oracle Linux the default user is
`opc` (not `ubuntu`, despite what older docs say):

```bash
ssh -i ~/.ssh/adm_key opc@155.248.184.176
```

Once on the box, switch to the service user when working with the stack:

```bash
sudo su - adm
```

## 2. First-boot check (important)

Cloud-init clones this repo to `/opt/adm/repo` and runs
`deploy/scripts/setup.sh`, which builds the Docker Compose stack, pulls the
Ollama model, and installs an `adm.service` systemd unit.

**Known caveat:** `deploy/terraform/cloud-init.yaml` installs `docker.io` and
`docker-compose-plugin` — Ubuntu package names that do not exist on Oracle
Linux 8. If first boot could not install Docker, the stack will not be
running. Verify and, if needed, bootstrap manually:

```bash
# Is docker present and the stack up?
docker ps 2>/dev/null || echo "Docker missing - bootstrap needed"

# Manual bootstrap on Oracle Linux 8:
sudo dnf install -y dnf-utils git curl jq
sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker adm

# Then run the ADM setup script:
sudo mkdir -p /opt/adm && sudo chown adm:adm /opt/adm
sudo -u adm git clone https://github.com/Jest-Test-Team/Agentic-Defense-Matrix-ADM-.git /opt/adm/repo || true
sudo bash /opt/adm/repo/deploy/scripts/setup.sh
```

**Memory caveat:** the micro shape has 1 GB RAM. Before building, add swap or
the Go/Rust image builds may be OOM-killed:

```bash
sudo dd if=/dev/zero of=/swapfile bs=1M count=4096
sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

## 3. Operating the stack

Management scripts live in `/opt/adm/repo/deploy/scripts/` (run as `adm`):

```bash
sudo -u adm /opt/adm/repo/deploy/scripts/status.sh    # containers + health + resources
sudo -u adm /opt/adm/repo/deploy/scripts/logs.sh      # tail service logs
sudo -u adm /opt/adm/repo/deploy/scripts/restart.sh   # restart the stack
sudo -u adm /opt/adm/repo/deploy/scripts/update.sh    # git pull + rebuild + restart
```

Or drive Docker Compose directly from `/opt/adm/repo`:

```bash
cd /opt/adm/repo
docker compose ps                  # status
docker compose logs -f gateway     # follow one service
docker compose up -d               # start
docker compose down                # stop
```

The stack also starts on boot via systemd: `sudo systemctl status adm`.

### Services in the stack

| Service | Port | Purpose |
|---|---|---|
| gateway | 8080 (public), 9090 gRPC | API front door; routes chat/tool calls through policy + SIEM |
| ollama | 11434 (public) | Local LLM runtime |
| redis | 6379 (internal) | Session and event store |
| siem | 9091 (internal) | Security event ingestion/retention |
| policy | 8181 (internal) | OPA policy engine |
| planner / executor / summarizer | 9081–9083 gRPC (internal) | Agent services |
| watchdog | host network | Endpoint monitor (Rust) |
| otel-collector | 4317/4318 (internal) | Telemetry |
| control-plane | 9092 (internal) | Auto-update checks |

## 4. Using the Gateway API (port 8080)

Health, readiness, and version:

```bash
curl http://155.248.184.176:8080/v1/health
curl http://155.248.184.176:8080/v1/ready
curl http://155.248.184.176:8080/v1/version
```

Chat completion (OpenAI-compatible shape, served by the local Ollama model):

```bash
curl -X POST http://155.248.184.176:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.1:8b",
    "messages": [{"role": "user", "content": "Summarize the last security events."}]
  }'
```

Tool execution (runs through the policy engine and executor):

```bash
curl -X POST http://155.248.184.176:8080/v1/tools/execute \
  -H "Content-Type: application/json" \
  -d '{"tool": "<tool-name>", "arguments": {}}'
```

Admin endpoints:

```bash
curl http://155.248.184.176:8080/v1/admin/sessions          # list sessions
curl http://155.248.184.176:8080/v1/admin/metrics           # metrics
curl -X POST http://155.248.184.176:8080/v1/admin/revoke/<session_id>
curl -X POST http://155.248.184.176:8080/v1/admin/update/check
```

> These endpoints are open to the internet on this deployment. Don't store
> anything sensitive on the instance, and consider restricting the NSG
> ingress rules to your own IP (edit `deploy/terraform/main.tf`,
> `adm_nsg_ingress`) once you're past experimentation.

### Analysis API (port 8090, or `https://<domain>` behind Caddy)

The Rust analysis engine powers the dashboard and exposes the live telemetry:

```bash
curl https://api.dennisleehappy.org/api/stats        # scoreboard (block/detection/landing, MTTR, residual risk)
curl https://api.dennisleehappy.org/api/timeline      # recent attack→remediation sessions
curl https://api.dennisleehappy.org/api/chains        # successful attack chains (+ /api/chains/:id)
curl https://api.dennisleehappy.org/api/system        # every stack component's health + hints
curl https://api.dennisleehappy.org/api/llm           # LLM providers: Groq (primary) / X.AI (fallback), which is active
curl https://api.dennisleehappy.org/api/latency       # δ (detection) & κ (containment) distributions + CDF
curl "https://api.dennisleehappy.org/api/search?q=team:red+AND+outcome:allowed"   # full-text over events (Elastic)
curl https://api.dennisleehappy.org/api/stream        # live SSE event feed
```

`/api/latency` is the deployed δ/κ instrument (see [research](research/latency-results.md)):
`delta_detection` and `kappa_containment`, each with `p50/p95/p99` and a `cdf` array.

## 5. Using Ollama directly (port 11434)

```bash
curl http://155.248.184.176:11434/api/tags        # list installed models
curl http://155.248.184.176:11434/api/generate \
  -d '{"model": "qwen2.5:0.5b", "prompt": "hello", "stream": false}'
```

**Model sizing matters on this instance.** The compose defaults assume
`llama3.1:8b`, which needs ~6 GB RAM and will not run on the 1 GB micro.
Until the instance is upgraded to A1, pull a tiny model instead:

```bash
docker exec adm-ollama ollama pull qwen2.5:0.5b   # ~400 MB, fits the micro
```

and pass that model name in API calls (or update `ADM_MODEL` in
`docker-compose.yml`).

## 6. Upgrading to the A1 (ARM) shape

Tokyo's Always Free A1 capacity comes and goes. The workflow retries A1
automatically on each apply and only falls back to micro when Oracle reports
"Out of host capacity". To attempt an upgrade, re-dispatch the workflow
(Actions → Terraform OCI → Run workflow):

- `action=apply`, `auto_approve=true`, `allow_create_network=true`

or:

```bash
gh workflow run terraform-oci.yml -f action=apply -f auto_approve=true -f allow_create_network=true
```

If A1 capacity is available, Terraform replaces the instance with a
2 OCPU / 12 GB ARM box (same IP is *not* guaranteed). Off-peak JST hours
have the best odds. Once upgraded, `llama3.1:8b` still needs more RAM than
12 GB leaves free after the stack — `llama3.2:3b` or `qwen2.5:3b` are good
fits.

## 7. Redeploying, updating, destroying

- **Code/config changes** to `deploy/terraform/**` or the workflow file:
  push to `main` → the workflow runs a plan automatically. Applies always
  require a manual dispatch with `auto_approve=true`.
- **App updates on the instance**: `sudo -u adm /opt/adm/repo/deploy/scripts/update.sh`.
- **Destroy everything**: dispatch with `action=destroy`, `auto_approve=true`.

Every plan prints live tenancy diagnostics in the run log (`network_diagnostics`,
`compute_diagnostics`, `storage_diagnostics`, `quota_policies`) — read those
first when a run fails; they were built to explain exactly these failures.

## 8. Troubleshooting quick reference

| Symptom | Likely cause | Fix |
|---|---|---|
| `curl :8080` times out | Stack not running (cloud-init package mismatch on OL8) | Section 2 manual bootstrap |
| Gateway up, chat 500s | Model not pulled / too big for RAM | Pull a smaller model (Section 5) |
| Apply fails: `Out of host capacity` | No A1 hosts in Tokyo | Automatic micro fallback handles it; retry A1 later |
| Apply fails: `bootVolumeQuota` | Orphaned boot volumes eating the free-tier allowance | Dispatch with `cleanup_axiom_volumes=true`, or delete orphans in console (Block Storage → Boot Volumes, check every compartment) |
| Apply fails: `vcn-count` | VCN quota exhausted | See `network_diagnostics` in the plan log for what's consuming it |
| SSH refused | Wrong user | Use `opc@`, not `ubuntu@` |

## 9. Red team agent (running attacks against a deployment)

The OCI deployment above is the **defensive** stack only. The **red team
agent** is a separate component: a Go test harness in `tests/redteam/` that
fires 25 adversarial attack scenarios (RT001–RT025 — prompt injection, tool
chaining, RAG poisoning, reverse shell, container escape, egress
exfiltration, etc.) at a running ADM gateway and checks that the defenses
hold. It is **not** part of the deployed stack and does not run on the OCI
instance — you run it from your workstation or CI and point it at whatever
gateway you want to exercise.

### Target any deployment

The harness reads the gateway URL from the `ADM_GATEWAY_URL` environment
variable, defaulting to `http://localhost:8080`. Set it to run against a
remote deployment:

```bash
# Against the OCI instance
export ADM_GATEWAY_URL=http://155.248.184.176:8080

# Against a local stack
export ADM_GATEWAY_URL=http://localhost:8080
```

### Run it

```bash
# All red team scenarios (from the repo root)
make redteam
# equivalent to: go test -v -tags=redteam ./tests/redteam/...

# A single scenario
ADM_GATEWAY_URL=http://155.248.184.176:8080 \
  go test -v -run '^TestRT001_PromptInjection$' ./tests/redteam/...

# Build a standalone runner binary
make redteam-build   # -> build/redteam-runner
```

The scenarios also run automatically in CI via
`.github/workflows/red_team_fuzz.yml` (on every push/PR and weekly), where
they exercise a locally built gateway rather than a remote one.

### Deploying the red team agent elsewhere

Because it is just a Go test binary plus this repo, "deploying" it somewhere
else means running it from a host that can reach the target gateway:

1. **Prerequisites on the runner host**: Go 1.25+ and a clone of this repo
   (`git clone https://github.com/Jest-Test-Team/Agentic-Defense-Matrix-ADM-.git`).
2. **Point at the target**: `export ADM_GATEWAY_URL=http://<target-host>:8080`.
3. **Network path**: the target's port 8080 must be reachable from the
   runner. For the OCI instance that means the `adm-nsg` ingress rule on 8080
   (open to `0.0.0.0/0` by default) — if you tighten it to your own IP
   (recommended, see Section 4), run the red team agent from that same IP.
4. **Run** `make redteam` (or the standalone `build/redteam-runner`) and
   review the pass/fail output and any `fuzz*.json` / `fuzz*.log` artifacts.

> The model matters here too: the harness sends `"model": "llama3.1:8b"`. If
> your target runs a smaller model (as the micro instance must — Section 5),
> some scenarios may respond differently. Adjust the model in
> `tests/redteam/harness.go` (`ChatRequest.Model`) to match the target.

> **Authorization:** only run the red team agent against ADM deployments you
> own or are explicitly authorized to test. It sends real attack payloads.

### Continuous battle red/green agents (deployed stack)

On a full battle deploy (`make battle-up` / `battle-up.sh`), `adm-redteam` and
`adm-greenteam` run as long-lived containers (not the Go test harness above):

| Env | Meaning |
|-----|---------|
| `ADM_RED_LLM` | `true` → on landing, call Groq for adaptive next step (default in battle compose) |
| `ADM_GREEN_LLM` | `true` → LLM triage + SOC summary before revoke/restart |
| `ADM_CHAIN_MAX_STEPS` | Max adaptive follow-ups per chain (default 5) |
| `ADM_LLM_*` | Same Groq → X.AI keys as the gateway (see ADR-006 / ADR-008) |

Smoke-check chains after landings:

```bash
curl -s "http://$IP:8090/api/chains?status=landed&limit=5" | jq .
curl -s "http://$IP:8090/api/chains/<chain-uuid>" | jq .
```

## 10. Research experiments (offline, from the repo root)

The [research program](research/) ships runnable experiments that produce the
paper's numbers from the deterministic corpus — no cloud needed:

```bash
go run ./cmd/ablation      # C1: embedding-φ vs keyword detection by mutation class
go run ./cmd/sweep         # C1: window-W sweep vs the Eq. 2/3 exponential bounds
go run ./cmd/latency       # C2: δ (detection) & κ (containment primitive) timing
go run ./cmd/overhead      # C2: lock-free vs mutex SIEM hot path, the ≤5% claim
go run ./cmd/baseline      # SOTA: ADM drift vs Llama Guard (add ADM_LLM_API_KEY for the α measurement)
go run ./cmd/corpus_dump   # regenerate the 10,000-variant catalog (dashboard/public/corpus.json)
```

Add `-json` to any of them for machine-readable output. Results and the rendered
figures are in [`docs/research/`](research/) (`figures.html` is a published
interactive artifact). Live δ/κ come from `GET /api/latency` (Section 4).

