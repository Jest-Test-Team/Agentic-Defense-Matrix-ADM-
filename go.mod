module github.com/adm

go 1.22.0

require (
	github.com/docker/docker v27.1.1+incompatible
	github.com/go-redis/redis/v9 v9.6.1
	github.com/labstack/echo/v4 v4.12.0
	github.com/open-policy-agent/opa v1.0.0
	go.opentelemetry.io/otel v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0
	go.opentelemetry.io/otel/sdk v1.28.0
	go.uber.org/zap v1.27.0
	google.golang.org/grpc v1.65.0
	google.golang.org/protobuf v1.34.2
	gopkg.in/yaml.v3 v3.0.1
)
