package otel

import (
	"context"
	"os"

	"github.com/uptrace/uptrace-go/uptrace"
)

func InitUptrace() {
	uptrace.ConfigureOpentelemetry(
		uptrace.WithDSN(os.Getenv("UPTRACE_DSN")),
		uptrace.WithServiceName("conxole-relay"),
		uptrace.WithServiceVersion("v1.0.0"),
		uptrace.WithDeploymentEnvironment("local"),
	)
}

func ShutdownUptrace(ctx context.Context) error {
	return uptrace.Shutdown(ctx)
}
