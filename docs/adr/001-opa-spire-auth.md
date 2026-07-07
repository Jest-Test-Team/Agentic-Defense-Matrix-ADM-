# ADR-001: Use OPA + SPIRE for Authentication and Policy Enforcement

## Status

Accepted

## Context

ADM requires dynamic, fine-grained access control for agent services that can adapt to threat levels in real-time. Traditional static IAM (AWS IAM roles) is insufficient for ephemeral agent containers that need per-request permission evaluation.

## Decision

Use **Open Policy Agent (OPA)** for policy-as-code evaluation and **SPIRE** for workload identity (mTLS + SVIDs).

### Why OPA?
- Rego language enables expressive policy authoring
- Sidecar/embedded deployment fits container architecture
- Hot-reloadable policies without service restart
- Rich ecosystem and CNCF maturity

### Why SPIRE?
- Automatic workload identity via SPIFFE IDs
- Short-lived X.509 certificates (SVIDs)
- No pre-shared keys or static credentials
- Cross-platform support (Linux, macOS, Windows)

## Consequences

- Policy logic lives in Rego files, version-controlled alongside code
- Gateway validates SVIDs on every request
- SIEM can trigger dynamic policy updates (e.g., block compromised sessions)
- Learning curve for Rego syntax for team members

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| AWS IAM | Cloud lock-in, no local evaluation |
| HashiCorp Vault | Heavier operational overhead, not purpose-built for policy |
| Cedar (AWS) | Less mature ecosystem, vendor-specific |
| Custom RBAC | Reimplementing solved problems |
