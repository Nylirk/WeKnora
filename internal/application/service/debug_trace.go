package service

import (
	"context"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// debugTraceService is the in-memory ring buffer for HTTP debug traces.
type debugTraceService struct {
	mu            sync.RWMutex
	entries       []*types.HTTPDebugTrace
	settingSvc    interfaces.SystemSettingService
	maxEntriesDef int64
	ttlMinutesDef int64
}

// NewHTTPDebugTraceService creates a new in-memory trace ring buffer.
// Default sizes come from the caller; runtime overrides are read from
// systemSettings on each operation.
func NewHTTPDebugTraceService(
	settingSvc interfaces.SystemSettingService,
) interfaces.HTTPDebugTraceService {
	return &debugTraceService{
		entries:       make([]*types.HTTPDebugTrace, 0),
		settingSvc:    settingSvc,
		maxEntriesDef: 500,
		ttlMinutesDef: 60,
	}
}

func (s *debugTraceService) maxEntries(ctx context.Context) int {
	v := s.settingSvc.GetInt(ctx, "debug.http_trace.max_entries", "", s.maxEntriesDef)
	if v < 1 {
		return int(s.maxEntriesDef)
	}
	if v > 5000 {
		return 5000
	}
	return int(v)
}

func (s *debugTraceService) ttl(ctx context.Context) time.Duration {
	v := s.settingSvc.GetInt(ctx, "debug.http_trace.ttl_minutes", "", s.ttlMinutesDef)
	if v < 1 {
		v = s.ttlMinutesDef
	}
	if v > 1440 {
		v = 1440
	}
	return time.Duration(v) * time.Minute
}

// Record appends a trace, evicting the oldest entry when over capacity.
func (s *debugTraceService) Record(ctx context.Context, entry *types.HTTPDebugTrace) {
	if entry == nil {
		return
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	max := s.maxEntries(ctx)
	s.entries = append(s.entries, entry)
	if len(s.entries) > max {
		// Evict oldest entries
		excess := len(s.entries) - max
		s.entries = s.entries[excess:]
	}
}

// List returns non-expired traces, newest first.
func (s *debugTraceService) List(ctx context.Context) []*types.HTTPDebugTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ttl := s.ttl(ctx)
	cutoff := time.Now().Add(-ttl)

	// Collect non-expired, then reverse for newest-first
	live := make([]*types.HTTPDebugTrace, 0, len(s.entries))
	for _, e := range s.entries {
		if e.CompletedAt.After(cutoff) {
			live = append(live, e)
		}
	}

	// Reverse to newest-first
	for i, j := 0, len(live)-1; i < j; i, j = i+1, j-1 {
		live[i], live[j] = live[j], live[i]
	}

	return live
}

// Get returns a single trace by ID.
func (s *debugTraceService) Get(ctx context.Context, id string) *types.HTTPDebugTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, e := range s.entries {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// Clear removes all entries.
func (s *debugTraceService) Clear(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := len(s.entries)
	s.entries = make([]*types.HTTPDebugTrace, 0)
	logger.Infof(ctx, "[debug_trace] cleared %d trace entries", count)
}
