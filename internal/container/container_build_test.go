package container

import (
	"testing"

	"go.uber.org/dig"
)

func TestBuildContainerResolvesEvaluationReconciliationDependencies(t *testing.T) {
	tests := []struct {
		name      string
		redisAddr string
	}{
		{name: "sync mode"},
		{name: "redis mode", redisAddr: "127.0.0.1:6379"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REDIS_ADDR", tt.redisAddr)
			BuildContainer(dig.New(dig.DryRun(true)))
		})
	}
}
