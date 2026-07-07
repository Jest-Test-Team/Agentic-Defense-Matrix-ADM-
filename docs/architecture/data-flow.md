# Data Flow Architecture

## Event Ingestion Pipeline

```mermaid
flowchart LR
    subgraph "Event Sources"
        GW[Gateway]
        WD[Watchdog]
        AG[Agents]
    end

    subgraph "Ingestion"
        SIEM[SIEM Engine]
        RB[Ring Buffer<br/>64K events]
    end

    subgraph "Storage Tiers"
        REDIS[(Redis Streams<br/>Hot: 7d)]
        WARM[Warm Storage<br/>JSONL: 30d]
        COLD[(Cold Storage<br/>S3: 180d)]
    end

    subgraph "Processing"
        CORR[Correlation Engine]
        RULES[Rule Evaluator]
        ALERT[Alert Dispatcher]
    end

    subgraph "Response"
        WEBHOOK[Webhook → Gateway]
        KILL[Kill Container]
        BLOCK[Block Egress]
    end

    GW -->|OTLP| SIEM
    WD -->|Syscall Events| SIEM
    AG -->|Tool Results| SIEM
    SIEM --> RB
    RB --> REDIS
    REDIS -->|7d retention| WARM
    WARM -->|30d retention| COLD
    REDIS --> CORR
    CORR --> RULES
    RULES -->|Threshold met| ALERT
    ALERT --> WEBHOOK
    ALERT --> KILL
    ALERT --> BLOCK
```

## Semantic Analysis Pipeline

```mermaid
flowchart TD
    A[User Prompt] --> B[Tokenize]
    B --> C[Embed<br/>Local Model]
    C --> D[Similarity Check<br/>vs Known Patterns]
    D --> E{Score > Threshold?}
    E -->|Clean| F[Pass to OPA]
    E -->|Suspicious| G[Rate Limit]
    E -->|Malicious| H[Block + Alert SIEM]
    F --> I[OPA Policy Check]
    I -->|Allowed| J[Route to Agent]
    I -->|Denied| K[Block + Log]
```

## Agent Execution Pipeline

```mermaid
flowchart TD
    A[Plan Request] --> B[Planner Agent]
    B --> C[LLM: Generate Steps]
    C --> D[Return PlannedStep[]]

    D --> E{For Each Step}
    E --> F[Executor Agent]
    F --> G[Create Ephemeral Container]
    G --> H[Mount Tool Schema]
    H --> I[Execute Tool Call]
    I --> J[Watchdog: Monitor Syscalls]
    J --> K{Anomaly?}
    K -->|No| L[Collect Result]
    K -->|Yes| M[Kill + Alert]
    L --> N[Report to Gateway]

    N --> O[Summarizer Agent]
    O --> P[LLM: Generate Summary]
    P --> Q[Return Response]
```

## Green Team Auto-Response

```mermaid
sequenceDiagram
    participant SIEM as SIEM Engine
    participant GW as Gateway
    participant WD as Watchdog
    participant Docker as Docker API

    Note over SIEM: Correlation rule triggered

    SIEM->>GW: Webhook: Revoke Token
    GW->>GW: Invalidate JWT
    GW->>WD: Kill Session
    WD->>Docker: SIGKILL container
    WD->>WD: Block all egress
    WD->>SIEM: Report remediation
    SIEM->>SIEM: Log alert resolved
```
