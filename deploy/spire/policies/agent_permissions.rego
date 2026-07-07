# ADM OPA Policy Bundle
# Agent permissions and egress control policies

package adm.agent.permissions

default allow = false

# Agent role definitions
agent_roles := {
    "planner": {
        "tools": ["read_file", "list_directory", "query_knowledge_base"],
        "max_tokens": 4096,
        "max_requests_per_minute": 60,
    },
    "executor": {
        "tools": ["run_command", "http_request", "write_file"],
        "max_tokens": 8192,
        "max_requests_per_minute": 30,
        "sandbox_required": true,
    },
    "summarizer": {
        "tools": ["read_conversation"],
        "max_tokens": 2048,
        "max_requests_per_minute": 20,
    },
}

# Allow if agent role exists and tool is permitted
allow {
    agent := agent_roles[input.agent_role]
    tool := input.tool_name
    agent.tools[_] == tool
}

# Rate limit check
allow {
    agent := agent_roles[input.agent_role]
    input.request_count <= agent.max_requests_per_minute
}

# Egress policy
package adm.agent.egress

default egress_allowed = false

# Approved external endpoints per agent role
approved_endpoints := {
    "planner": [],
    "executor": [
        "api.github.com",
        "httpbin.org",
    ],
    "summarizer": [],
}

egress_allowed {
    agent := approved_endpoints[input.agent_role]
    host := input.destination_host
    agent[_] == host
}

# Block all by default for unplanned destinations
deny[msg] {
    not egress_allowed
    msg := sprintf("Egress to %s not allowed for agent role %s", [input.destination_host, input.agent_role])
}

# Session limits
package adm.session.limits

default session_valid = false

session_valid {
    input.session_age_seconds < 3600
    input.token_expiry > time.now_ns()
}

deny[msg] {
    not session_valid
    msg := "Session expired or token invalid"
}
