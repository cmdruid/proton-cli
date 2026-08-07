// Package progress defines the contract between long-running domain work and
// whatever reports it. Services take a Sink so they never depend on the
// presentation layer; the ui package supplies the implementation that draws a
// bar, and Nop covers every other caller.
package progress

// Sink receives byte-transfer progress for one operation.
//
// Start is called once before any Add, Done once after the last one. An
// implementation must tolerate a zero total (unknown size) and a running count
// that exceeds total, which happens whenever encryption adds per-block
// overhead to the source size.
type Sink interface {
	Start(total int64, label string)
	Add(n int64)
	Done()
}

// Nop discards everything. It is the zero-value Sink, so a nil-safe caller can
// use Of to avoid branching.
type Nop struct{}

func (Nop) Start(int64, string) {}
func (Nop) Add(int64)           {}
func (Nop) Done()               {}

// Of returns s, or a Nop when s is nil, so callers never nil-check.
func Of(s Sink) Sink {
	if s == nil {
		return Nop{}
	}
	return s
}
