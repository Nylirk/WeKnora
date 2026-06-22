package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// HTTPDebugTraceService manages the in-memory ring buffer for HTTP debug traces.
// All methods are thread-safe. The service reads runtime configuration from
// SystemSettingService on every request; there is no startup binding.
type HTTPDebugTraceService interface {
	// Record appends a trace entry. When the ring buffer is full the oldest
	// entry is silently evicted. The entry's ID is generated here.
	Record(ctx context.Context, entry *types.HTTPDebugTrace)

	// List returns all non-expired traces, newest first. Expired traces
	// (CompletedAt + TTL < now) are filtered out during this call.
	List(ctx context.Context) []*types.HTTPDebugTrace

	// Get returns a single trace by ID, or nil.
	Get(ctx context.Context, id string) *types.HTTPDebugTrace

	// Clear removes all entries from the ring buffer.
	Clear(ctx context.Context)
}
