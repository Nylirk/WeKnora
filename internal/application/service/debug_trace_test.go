package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// mockSystemSettingService is a minimal mock for debug trace tests.
type mockSystemSettingService struct {
	ints  map[string]int64
	bools map[string]bool
}

func (m *mockSystemSettingService) GetInt(_ context.Context, key, _ string, def int64) int64 {
	if v, ok := m.ints[key]; ok {
		return v
	}
	return def
}

func (m *mockSystemSettingService) GetBool(_ context.Context, key, _ string, def bool) bool {
	if v, ok := m.bools[key]; ok {
		return v
	}
	return def
}

func (m *mockSystemSettingService) GetString(_ context.Context, key, _ string, def string) string {
	return def
}

func (m *mockSystemSettingService) GetStringList(_ context.Context, key, _ string, def []string) []string {
	return def
}

func (m *mockSystemSettingService) List(_ context.Context) ([]*types.SystemSetting, error) {
	return nil, nil
}

func (m *mockSystemSettingService) Get(_ context.Context, key string) (*types.SystemSetting, error) {
	return nil, nil
}

func (m *mockSystemSettingService) Update(_ context.Context, key string, _ any) (*types.SystemSetting, error) {
	return nil, nil
}

func (m *mockSystemSettingService) Reset(_ context.Context, key string) error {
	return nil
}

func (m *mockSystemSettingService) SubscribeRedis(_ context.Context) error {
	return nil
}

func TestRecordAndList(t *testing.T) {
	ctx := context.Background()
	svc := NewHTTPDebugTraceService(&mockSystemSettingService{
		ints:  map[string]int64{"debug.http_trace.max_entries": 3, "debug.http_trace.ttl_minutes": 60},
		bools: map[string]bool{},
	})

	now := time.Now()

	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "GET", Path: "/api/v1/test", Status: 200,
		StartedAt: now, CompletedAt: now,
	})
	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "POST", Path: "/api/v1/test", Status: 201,
		StartedAt: now, CompletedAt: now,
	})

	traces := svc.List(ctx)
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(traces))
	}
	// Newest first
	if traces[0].Method != "POST" {
		t.Errorf("expected newest first (POST), got %s", traces[0].Method)
	}
}

func TestRingBufferEviction(t *testing.T) {
	ctx := context.Background()
	svc := NewHTTPDebugTraceService(&mockSystemSettingService{
		ints:  map[string]int64{"debug.http_trace.max_entries": 2, "debug.http_trace.ttl_minutes": 60},
		bools: map[string]bool{},
	})

	now := time.Now()

	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "A", Path: "/a", Status: 200,
		StartedAt: now, CompletedAt: now,
	})
	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "B", Path: "/b", Status: 200,
		StartedAt: now, CompletedAt: now,
	})
	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "C", Path: "/c", Status: 200,
		StartedAt: now, CompletedAt: now,
	})

	traces := svc.List(ctx)
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces after eviction, got %d", len(traces))
	}
	// A should be evicted; B and C remain
	for _, tr := range traces {
		if tr.Method == "A" {
			t.Errorf("oldest entry A should have been evicted")
		}
	}
}

func TestTTLExpiration(t *testing.T) {
	ctx := context.Background()
	svc := NewHTTPDebugTraceService(&mockSystemSettingService{
		ints:  map[string]int64{"debug.http_trace.max_entries": 10, "debug.http_trace.ttl_minutes": 1},
		bools: map[string]bool{},
	})

	now := time.Now()
	expired := now.Add(-2 * time.Minute)

	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "OLD", Path: "/old", Status: 200,
		StartedAt: expired, CompletedAt: expired,
	})
	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "NEW", Path: "/new", Status: 200,
		StartedAt: now, CompletedAt: now,
	})

	traces := svc.List(ctx)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace after TTL filter, got %d", len(traces))
	}
	if traces[0].Method != "NEW" {
		t.Errorf("expected NEW to survive, got %s", traces[0].Method)
	}
}

func TestGetByID(t *testing.T) {
	ctx := context.Background()
	svc := NewHTTPDebugTraceService(&mockSystemSettingService{
		ints:  map[string]int64{"debug.http_trace.max_entries": 10, "debug.http_trace.ttl_minutes": 60},
		bools: map[string]bool{},
	})

	now := time.Now()
	entry := &types.HTTPDebugTrace{
		Method: "GET", Path: "/api/v1/xyz", Status: 200,
		StartedAt: now, CompletedAt: now,
	}
	svc.Record(ctx, entry)

	found := svc.Get(ctx, entry.ID)
	if found == nil {
		t.Fatal("expected to find trace by ID")
	}
	if found.ID != entry.ID {
		t.Errorf("expected ID %s, got %s", entry.ID, found.ID)
	}

	missing := svc.Get(ctx, "nonexistent")
	if missing != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestClear(t *testing.T) {
	ctx := context.Background()
	svc := NewHTTPDebugTraceService(&mockSystemSettingService{
		ints:  map[string]int64{"debug.http_trace.max_entries": 10, "debug.http_trace.ttl_minutes": 60},
		bools: map[string]bool{},
	})

	now := time.Now()
	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "GET", Path: "/test", Status: 200,
		StartedAt: now, CompletedAt: now,
	})
	svc.Record(ctx, &types.HTTPDebugTrace{
		Method: "POST", Path: "/test", Status: 201,
		StartedAt: now, CompletedAt: now,
	})

	svc.Clear(ctx)
	traces := svc.List(ctx)
	if len(traces) != 0 {
		t.Fatalf("expected 0 traces after clear, got %d", len(traces))
	}
}
