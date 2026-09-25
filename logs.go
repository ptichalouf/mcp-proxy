package main

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Log stream labels. stdout/stderr are what the downstream process wrote;
// system is the proxy's own account of a server's lifecycle.
const (
	logStreamStdout = "stdout"
	logStreamStderr = "stderr"
	logStreamSystem = "system"
)

// logRingSize is how many lines are kept per server. A downstream that is
// spewing must not be able to grow the proxy's memory without bound, and the
// UI only ever shows a recent tail, so old lines are dropped rather than
// paged.
const logRingSize = 500

// logEntry is one line as the UI consumes it.
type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Stream    string `json:"stream,omitempty"`
	Source    string `json:"source,omitempty"`
}

// logStore keeps a bounded ring of recent lines per server.
//
// It exists because stdio downstreams write their diagnostics to stderr, which
// otherwise only reaches the proxy's debug log - by the time someone asks "why
// is this server failing?", it is gone. Keeping a short tail in memory is what
// lets the UI answer that question without a log aggregator.
type logStore struct {
	mu      sync.Mutex
	entries map[string][]logEntry
}

func newLogStore() *logStore {
	return &logStore{entries: make(map[string][]logEntry)}
}

func (s *logStore) append(server string, entry logEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := s.entries[server]
	if len(ring) >= logRingSize {
		// Drop the oldest by reslicing and copying down; the ring stays a
		// plain slice so reads need no index arithmetic.
		copy(ring, ring[len(ring)-logRingSize+1:])
		ring = ring[:logRingSize-1]
	}
	s.entries[server] = append(ring, entry)
}

// tail returns a copy of the recent lines for a server, oldest first.
func (s *logStore) tail(server string, limit int) []logEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := s.entries[server]
	if limit > 0 && len(ring) > limit {
		ring = ring[len(ring)-limit:]
	}
	out := make([]logEntry, len(ring))
	copy(out, ring)
	return out
}

// forget drops a removed server's history, so uninstalling and reinstalling
// under the same name does not resurrect the old server's output.
func (s *logStore) forget(server string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, server)
}

// activeLogStore is the store the management API reads from. It is a package
// global because the places that produce lines - the stderr drain goroutine,
// the supervisor - are reached from code paths that have no handle on the
// Manager, and threading one through would touch every signature for a feature
// that is off by default.
//
// It stays nil unless the management API is enabled, and recordServerLog is a
// no-op in that case, so a proxy started without -web keeps exactly its
// previous behaviour and allocates nothing.
var activeLogStore atomic.Pointer[logStore]

// recordServerLog files one line against a server. Safe to call at any time,
// including before (or without) the management API being enabled.
func recordServerLog(server, stream, level, message string) {
	store := activeLogStore.Load()
	if store == nil {
		return
	}
	message = strings.TrimRight(message, "\r\n")
	if message == "" {
		return
	}
	store.append(server, logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   message,
		Stream:    stream,
		Source:    server,
	})
}
