# OPA (Open Policy Agent) Dockerfile
FROM openpolicyagent/opa:latest

# Default policy directory is mounted via volume
EXPOSE 8181

ENTRYPOINT ["opa"]
CMD ["run", "--server", "--addr=0.0.0.0:8181", "--log-level=info", "/policies"]
