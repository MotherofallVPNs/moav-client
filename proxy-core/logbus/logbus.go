// Package logbus is an in-process pub/sub log bus that wraps the stdlib
// logger. Every log line written by proxy-core (or sing-box stderr captured
// elsewhere) lands in a bounded ring buffer with a parsed level + timestamp,
// AND is fanned out over a channel to live subscribers so the API can
// stream events to WebSocket clients.
package logbus

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"time"
)

// Event is one structured log record.
type Event struct {
	Ts      int64  `json:"ts"`      // unix-ms
	Level   string `json:"level"`   // info | warn | error
	Source  string `json:"source"`  // "proxy-core" | "singbox" | ...
	Message string `json:"message"`
}

// Bus is a fan-out hub plus per-level ring buffers.
//
// Why per-level: under heavy INFO traffic (probe results, sidecar logs)
// a single shared ring rolls warn/error events out within seconds and
// the operator misses them when they later open the Debug tab. With
// separate rings, the most recent N warns and N errors are ALWAYS in
// scrollback no matter how spammy the INFO stream is.
type Bus struct {
	mu        sync.RWMutex
	infoRing  []Event
	warnRing  []Event
	errorRing []Event
	maxPer    int
	subsMu    sync.RWMutex
	subs      map[chan Event]struct{}
}

// New creates a bus retaining "ring" events per level (info / warn / error).
// Total memory cap is ~3 * ring entries.
func New(ring int) *Bus {
	if ring <= 0 {
		ring = 500
	}
	return &Bus{
		infoRing:  make([]Event, 0, ring),
		warnRing:  make([]Event, 0, ring),
		errorRing: make([]Event, 0, ring),
		maxPer:    ring,
		subs:      make(map[chan Event]struct{}),
	}
}

// Publish appends and fans out an event.
func (b *Bus) Publish(ev Event) {
	if ev.Ts == 0 {
		ev.Ts = time.Now().UnixMilli()
	}
	if ev.Level == "" {
		ev.Level = "info"
	}
	b.mu.Lock()
	switch ev.Level {
	case "error":
		b.errorRing = pushRing(b.errorRing, ev, b.maxPer)
	case "warn":
		b.warnRing = pushRing(b.warnRing, ev, b.maxPer)
	default:
		b.infoRing = pushRing(b.infoRing, ev, b.maxPer)
	}
	b.mu.Unlock()

	b.subsMu.RLock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // slow subscriber, drop
		}
	}
	b.subsMu.RUnlock()
}

func pushRing(buf []Event, ev Event, max int) []Event {
	if len(buf) >= max {
		copy(buf, buf[1:])
		buf[len(buf)-1] = ev
	} else {
		buf = append(buf, ev)
	}
	return buf
}

// Snapshot returns a copy of every ring, interleaved by timestamp (oldest
// first). Both newly-arrived warns and a long history of info events are
// represented; warns/errors don't get crowded out by info spam.
func (b *Bus) Snapshot() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	merged := make([]Event, 0, len(b.infoRing)+len(b.warnRing)+len(b.errorRing))
	merged = append(merged, b.infoRing...)
	merged = append(merged, b.warnRing...)
	merged = append(merged, b.errorRing...)
	// Sort by timestamp ascending. The ring buffers are themselves
	// time-ordered per-level, but a merge needs a global re-sort.
	sortEventsByTs(merged)
	return merged
}

// sortEventsByTs sorts ev in place by Ts ascending. Inline insertion sort
// because the slice is already mostly sorted (each ring is ordered) and N
// is small.
func sortEventsByTs(ev []Event) {
	for i := 1; i < len(ev); i++ {
		j := i
		for j > 0 && ev[j-1].Ts > ev[j].Ts {
			ev[j-1], ev[j] = ev[j], ev[j-1]
			j--
		}
	}
}

// Subscribe returns a channel for incoming events and a function to release it.
func (b *Bus) Subscribe(buf int) (<-chan Event, func()) {
	if buf <= 0 {
		buf = 16
	}
	ch := make(chan Event, buf)
	b.subsMu.Lock()
	b.subs[ch] = struct{}{}
	b.subsMu.Unlock()
	return ch, func() {
		b.subsMu.Lock()
		delete(b.subs, ch)
		b.subsMu.Unlock()
		close(ch)
	}
}

// CapturingWriter is an io.Writer that splits incoming bytes by newline and
// publishes each line to a Bus. Plug it into log.SetOutput to capture every
// stdlib log call. We keep a passthrough so the original stderr still shows
// the log line — useful for `docker logs`.
type CapturingWriter struct {
	Bus        *Bus
	Source     string
	Passthrough []byte // assigned externally; CapturingWriter.Write writes to this too
	pass        func(p []byte) (int, error)
	buf         bytes.Buffer
	mu          sync.Mutex
}

// NewCapturingWriter wires a Bus and a passthrough write function (typically
// os.Stderr.Write so docker logs still works).
func NewCapturingWriter(bus *Bus, source string, pass func(p []byte) (int, error)) *CapturingWriter {
	return &CapturingWriter{Bus: bus, Source: source, pass: pass}
}

func (w *CapturingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadBytes('\n')
		if err != nil {
			// no full line yet; put what we read back
			w.buf.Write(line)
			break
		}
		s := strings.TrimRight(string(line), "\r\n")
		if s != "" {
			// Strip the stdlib "2026/.. .." date prefix BEFORE classifying —
			// classifyLevel's prefix rules ("probe ", "fatal:", …) assume a
			// clean line. Classifying the raw date-prefixed line silently
			// dropped every rule to the info default.
			clean := stripDatePrefix(s)
			w.Bus.Publish(Event{
				Level:   classifyLevel(clean),
				Source:  w.Source,
				Message: clean,
			})
		}
	}
	w.mu.Unlock()

	if w.pass != nil {
		return w.pass(p)
	}
	return len(p), nil
}

// classifyLevel inspects an unstructured log line and returns info/warn/error.
//
// Rules of thumb:
//   - **error** is reserved for SYSTEM-level failures (fatal/panic, no
//     healthy endpoints anywhere, can't load config, can't bind a listener).
//     A single endpoint being unhealthy in a probe pass is NOT an error —
//     the balancer just routes around it.
//   - **warn** covers transient / recoverable conditions the operator
//     should notice (one dial failed and we're retrying, fall-back to
//     direct, deprecation notices).
//   - everything else (probe results, normal lifecycle, traffic) is **info**.
//
// Heuristics order matters — we check the most specific patterns first.
func classifyLevel(s string) string {
	low := strings.ToLower(s)

	// "probe cycle:" is our roll-up emitted at the end of each pass. We
	// classify it explicitly first so the embedded WARN keyword survives.
	if strings.HasPrefix(low, "probe cycle:") {
		if strings.Contains(low, "warn") {
			return "warn"
		}
		return "info"
	}

	// Per-endpoint probe lines: a failing probe (status=error/timeout) is a
	// warn so it surfaces in the Debug tab above the info stream; a healthy
	// probe stays info. It's never an error — one unhealthy peer isn't a
	// system failure, the balancer just routes around it.
	if strings.HasPrefix(low, "probe ") {
		if strings.Contains(low, "status=error") || strings.Contains(low, "status=timeout") {
			return "warn"
		}
		return "info"
	}

	switch {
	// Transient / recoverable conditions are warns even if they mention
	// "fail" / "no healthy" etc. — check these FIRST so the fallback log
	// lines don't bubble up to error.
	case strings.Contains(low, "warn") ||
		strings.Contains(low, "deprecat") ||
		strings.Contains(low, "falling back") ||
		strings.Contains(low, "fall back") ||
		strings.Contains(low, "trying next endpoint") ||
		strings.Contains(low, "failover") ||
		strings.Contains(low, "succeeded after") ||
		strings.Contains(low, "skipping") ||
		strings.Contains(low, "all candidates failed") || // we still try direct after
		strings.Contains(low, "no healthy endpoint") ||
		strings.Contains(low, "went unhealthy") || // status transition emitted by main.go
		strings.Contains(low, "recovered:") ||     // status transition emitted by main.go
		strings.Contains(low, "dial through ") && strings.Contains(low, "failed"):
		return "warn"

	// True system-level failures: process can't proceed.
	case strings.HasPrefix(low, "fatal:") ||
		strings.Contains(low, " fatal:") ||
		strings.HasPrefix(low, "panic:") ||
		strings.Contains(low, " panic:") ||
		strings.HasPrefix(low, "load config:") ||
		strings.HasPrefix(low, "could not load") ||
		strings.Contains(low, "listen:") && strings.Contains(low, "address already in use") ||
		strings.Contains(low, "listen:") && strings.Contains(low, "permission denied") ||
		strings.HasPrefix(low, "http listen:") ||
		strings.HasPrefix(low, "socks5 listen:") ||
		strings.HasPrefix(low, "api listen:"):
		return "error"

	default:
		return "info"
	}
}

// stripDatePrefix removes the stdlib "2026/06/03 00:00:00 " timestamp because
// we attach our own structured Ts. Keeps lines compact in the UI.
func stripDatePrefix(s string) string {
	if len(s) >= 20 && s[4] == '/' && s[7] == '/' && s[10] == ' ' {
		return strings.TrimSpace(s[20:])
	}
	return s
}

// Default is a process-global bus used when callers don't want to plumb one.
var Default = New(500)

// Helper functions for code that wants to publish directly.
func Info(source, msg string)  { Default.Publish(Event{Level: "info", Source: source, Message: msg}) }
func Warn(source, msg string)  { Default.Publish(Event{Level: "warn", Source: source, Message: msg}) }
func Error(source, msg string) { Default.Publish(Event{Level: "error", Source: source, Message: msg}) }

// CaptureStdLog wires the package-global stdlib logger so every log.Printf
// from anywhere in the process is mirrored to bus. Returns the previous
// output so callers can restore it.
func CaptureStdLog(bus *Bus, source string, pass func(p []byte) (int, error)) {
	log.SetOutput(NewCapturingWriter(bus, source, pass))
}
