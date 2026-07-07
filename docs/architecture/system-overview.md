# ADM System Architecture

## High-Level Overview

```mermaid
graph TB
    subgraph "External"
        User([User / Client])
        Ollama([Ollama LLM])
    end

    subgraph "ADM Control Plane"
        GW[Gateway<br/>Go + Echo]
        SIEM[SIEM Engine<br/>Go]
        CP[Control Plane<br/>Auto-Update]
    end

    subgraph "Agent Plane"
        P[Planner Agent<br/>Go]
        E[Executor Agent<br/>Go]
        S[Summarizer Agent<br/>Go]
    end

    subgraph "Policy Engine"
        OPA[OPA<br/>Rego Policies]
        SPIRE[SPIRE<br/>mTLS + SVIDs]
    end

    subgraph "Observability"
        OTel[OTel Collector]
        Redis[(Redis<br/>Streams + Hot Storage)]
    end

    subgraph "Endpoint Security"
        WD[Watchdog Daemon<br/>Rust]
        WFP[WFP Filters<br/>Windows]
        ES[Endpoint Security<br/>macOS]
    end

    User -->|HTTP/gRPC| GW
    GW -->|gRPC| P
    GW -->|gRPC| E
    GW -->|gRPC| S
    P -->|HTTP| Ollama
    E -->|HTTP| Ollama
    S -->|HTTP| Ollama
    GW -->|OTLP| OTel
    SIEM -->|OTLP| OTel
    SIEM -->|Read/Write| Redis
    GW -->|Evaluate| OPA
    SPIRE -->|SVID| GW
    WD -->|Events| OTel
    WD --> WFP
    WD --> ES
    CP -->|Version Check| User
```

## Request Flow

```mermaid
sequenceDiagram
    participant U as User
    participant GW as Gateway
    participant OPA as Policy Engine
    participant P as Planner
    participant E as Executor
    participant S as Summarizer
    participant O as Ollama
    participant WD as Watchdog
    participant SIEM as SIEM

    U->>GW: POST /v1/chat/completions
    GW->>GW: Rate Limit Check
    GW->>GW: Semantic Analysis
    GW->>OPA: Evaluate Policy
    OPA-->>GW: Allow/Deny

    alt Allowed
        GW->>P: PlanExecution(prompt)
        P->>O: Chat(with tool schema)
        O-->>P: Tool call plan
        P-->>GW: PlannedStep[]

        loop For each step
            GW->>E: ExecuteTool(step)
            E->>WD: AddSessionFilter(session)
            WD-->>E: Filters Active
            E->>E: Execute in sandbox
            E->>WD: ReportSyscalls()
            WD->>SIEM: IngestEvent(syscall)
            E-->>GW: Result
        end

        GW->>S: Summarize(conversation)
        S->>O: Chat(summary prompt)
        O-->>S: Summary
        S-->>GW: Response
        GW-->>U: ChatCompletionResponse
    else Denied
        GW-->>U: 403 Forbidden
        GW->>SIEM: IngestEvent(denied)
    end
```

## Network Topology

```mermaid
graph LR
    subgraph "Public Network"
        Client([Client])
    end

    subgraph "Gateway DMZ"
        GW[Gateway :8080]
    end

    subgraph "Internal Network"
        SIEM[SIEM :9091]
        P[Planner :9081]
        E[Executor :9082]
        S[Summarizer :9083]
        OPA[OPA :8181]
        Redis[(Redis :6379)]
        Ollama[Ollama :11434]
    end

    subgraph "Host Network"
        WD[Watchdog :unix-sock]
    end

    Client --> GW
    GW --> P
    GW --> E
    GW --> S
    GW --> OPA
    P --> Ollama
    E --> Ollama
    S --> Ollama
    SIEM --> Redis
    E --> WD
    WD --> SIEM
```

## Data Flow Diagram

```mermaid
flowchart TD
    A[User Request] --> B{Semantic Analysis}
    B -->|Clean| C[OPA Policy Check]
    B -->|Suspicious| D[Alert SIEM + Rate Limit]

    C -->|Allowed| E[Plan with LLM]
    C -->|Denied| F[Block + Log]

    E --> G[Execute Tool Calls]
    G --> H[Watchdog Monitors Syscalls]
    H --> I{Anomaly Detected?}

    I -->|No| J[Return Result]
    I -->|Yes| K[Green Team Response]
    K --> L[Revoke Token]
    K --> M[Kill Container]
    K --> N[Block Egress]

    J --> O[Summarize]
    O --> P[Response to User]
```

## Service Communication Matrix

| Source | Target | Protocol | Port | Purpose |
|--------|--------|----------|------|---------|
| Client | Gateway | HTTP/1.1, HTTP/2 | 8080 | Chat completions |
| Gateway | Planner | gRPC | 9081 | Task planning |
| Gateway | Executor | gRPC | 9082 | Tool execution |
| Gateway | Summarizer | gRPC | 9083 | Response summarization |
| Gateway | OPA | HTTP | 8181 | Policy evaluation |
| Planner | Ollama | HTTP | 11434 | LLM inference |
| Executor | Ollama | HTTP | 11434 | LLM inference |
| Summarizer | Ollama | HTTP | 11434 | LLM inference |
| Executor | Watchdog | Unix socket | — | Syscall reporting |
| Watchdog | SIEM | gRPC | 9091 | Event ingestion |
| SIEM | Redis | RESP | 6379 | Stream storage |
| All | OTel Collector | gRPC | 4317 | Traces/metrics |
| Control Plane | GitHub API | HTTPS | 443 | Auto-update check |
