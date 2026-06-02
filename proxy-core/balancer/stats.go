package balancer

import (
	"sync"
	"sync/atomic"
	"time"
)

// EndpointStat is the cumulative counters tracked for one endpoint id.
// Bytes are observed by the caller via Stats.RecordTraffic — the balancer
// itself only knows about dial outcomes.
type EndpointStat struct {
	ID            string `json:"id"`
	Dials         int64  `json:"dials"`
	DialErrors    int64  `json:"dial_errors"`
	Failovers     int64  `json:"failovers"`     // times another endpoint took over after this one failed
	BytesUp       int64  `json:"bytes_up"`
	BytesDown     int64  `json:"bytes_down"`
	LastDialUnix  int64  `json:"last_dial_unix"`
	LastErrorUnix int64  `json:"last_error_unix"`
	LastError     string `json:"last_error,omitempty"`
}

// Stats is a thread-safe per-endpoint counter store. It lives on the balancer
// because both DialContext and conn-wrapping callers in proxy/ need to update
// it. Fields are accessed via atomics where possible to keep the hot path
// lock-free.
type Stats struct {
	mu sync.RWMutex
	m  map[string]*EndpointStat
}

// NewStats creates a Stats tracker.
func NewStats() *Stats { return &Stats{m: make(map[string]*EndpointStat)} }

func (s *Stats) entry(id string) *EndpointStat {
	s.mu.RLock()
	e, ok := s.m[id]
	s.mu.RUnlock()
	if ok {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok = s.m[id]; ok {
		return e
	}
	e = &EndpointStat{ID: id}
	s.m[id] = e
	return e
}

// RecordDial logs a dial attempt; err may be nil.
func (s *Stats) RecordDial(id string, err error) {
	e := s.entry(id)
	atomic.AddInt64(&e.Dials, 1)
	atomic.StoreInt64(&e.LastDialUnix, time.Now().Unix())
	if err != nil {
		atomic.AddInt64(&e.DialErrors, 1)
		atomic.StoreInt64(&e.LastErrorUnix, time.Now().Unix())
		s.mu.Lock()
		e.LastError = err.Error()
		s.mu.Unlock()
	}
}

// RecordFailover increments the failover counter for an endpoint that
// failed and was replaced by another within the same DialContext call.
func (s *Stats) RecordFailover(id string) {
	atomic.AddInt64(&s.entry(id).Failovers, 1)
}

// RecordTraffic adds bytes to the counters. Called by the SOCKS5 / HTTP
// CONNECT tunnel goroutines as they io.Copy.
func (s *Stats) RecordTraffic(id string, up, down int64) {
	e := s.entry(id)
	if up > 0 {
		atomic.AddInt64(&e.BytesUp, up)
	}
	if down > 0 {
		atomic.AddInt64(&e.BytesDown, down)
	}
}

// Snapshot returns a copy of all stats.
func (s *Stats) Snapshot() []EndpointStat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EndpointStat, 0, len(s.m))
	for _, e := range s.m {
		out = append(out, *e)
	}
	return out
}
