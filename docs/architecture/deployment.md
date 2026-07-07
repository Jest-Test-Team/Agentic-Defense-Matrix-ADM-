# ADM Deployment Architecture

## Docker Compose Deployment

```mermaid
graph TB
    subgraph "Host Machine"
        subgraph "Docker Network: adm-internal"
            subgraph "Infrastructure"
                Redis[(Redis)]
                Ollama[Ollama<br/>GPU Passthrough]
                OTel[OTel Collector]
            end

            subgraph "Core Services"
                GW[Gateway]
                SIEM[SIEM]
                CP[Control Plane]
                OPA[OPA]
            end

            subgraph "Agent Services"
                P[Planner]
                E[Executor]
                S[Summarizer]
            end
        end

        subgraph "Docker Network: adm-public"
            GW
        end

        subgraph "Host Network"
            WD[Watchdog Daemon<br/>Privileged + PID Host]
        end

        subgraph "Volumes"
            RV[redis-data]
            OV[ollama-models]
            EV[executor-tmp]
        end
    end

    Redis --- RV
    Ollama --- OV
    E --- EV
```

## GHA Build Matrix

```mermaid
graph LR
    subgraph "GitHub Actions"
        CI[ci.yml]
        REL[release.yml]
        RT[red_team_fuzz.yml]
    end

    subgraph "Build Targets"
        W[windows/amd64]
        DM[darwin/amd64]
        DA[darwin/arm64]
        LA[linux/amd64]
    end

    subgraph "Artifacts"
        MSI[MSI Installer]
        PKG[.pkg Installer]
        TGZ[tar.gz Archive]
    end

    CI --> W & DM & DA & LA
    REL --> MSI & PKG & TGZ
    RT --> W & DM & DA & LA
```

## Installer Matrix

| Platform | Format | Service Manager | Install Path |
|----------|--------|----------------|--------------|
| Windows | MSI | Windows Service | `C:\Program Files\ADM\` |
| macOS | .pkg | launchd | `/Library/ADM/` |
| Linux | tar.gz | systemd | `/opt/adm/` |

## Service Endpoints

| Service | Protocol | Port | Health Check |
|---------|----------|------|--------------|
| Gateway | HTTP/gRPC | 8080/9090 | `GET /v1/health` |
| SIEM | gRPC/HTTP | 9091 | `GET /health` |
| OPA | HTTP | 8181 | `GET /health` |
| Planner | gRPC | 9081 | gRPC health probe |
| Executor | gRPC | 9082 | gRPC health probe |
| Summarizer | gRPC | 9083 | gRPC health probe |
| Control Plane | HTTP | 9092 | `GET /health` |
| Redis | RESP | 6379 | `redis-cli ping` |
| Ollama | HTTP | 11434 | `GET /api/tags` |
| OTel Collector | gRPC/HTTP | 4317/4318 | `:13133/health` |
