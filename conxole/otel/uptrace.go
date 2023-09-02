package otel

import (
	"context"
	"os"

	"github.com/uptrace/uptrace-go/uptrace"
)

func InitUptrace() {
	environment := "prod"
	if os.Getenv("PROD") == "false" {
		environment = "local"
	}

	uptrace.ConfigureOpentelemetry(
		uptrace.WithDSN(os.Getenv("UPTRACE_DSN")),
		uptrace.WithServiceName("conxole-relay"),
		uptrace.WithServiceVersion("v1.0.0"),
		uptrace.WithDeploymentEnvironment(environment),
	)
}

func ShutdownUptrace(ctx context.Context) error {
	return uptrace.Shutdown(ctx)
}
