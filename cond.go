package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// CondInt hold an int64 value that can be set arbitrarily.
type CondInt interface {
	Snapshot() CondInt
	Update(int64)
	Value() int64
	Writable() bool // 是否需要写入
}

// GetOrRegisterCondInt returns an existing Int or constructs and registers a
// new StandardContInt.
func GetOrRegisterCondInt(name string, r Registry, period time.Duration) CondInt {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewCondInt, period).(CondInt)
}

// NewCondInt constructs a new StandardCondInt.
func NewCondInt(period time.Duration) CondInt {
	return &StandardCondInt{0, period, time.Now()}
}

// CondIntSnapshot is a read-only copy of another Int.
type CondIntSnapshot struct {
	value    int64
	writable bool
}

// Snapshot returns the snapshot.
func (g *CondIntSnapshot) Snapshot() CondInt { return g }

// Update panics.
func (g *CondIntSnapshot) Update(int64) {
	panic("Update called on a IntSnapshot")
}

// Value returns the value at the time the snapshot was taken.
func (g *CondIntSnapshot) Value() int64 { return int64(g.value) }

// Writable returns the value should write to db
func (g *CondIntSnapshot) Writable() bool { return g.writable }

// StandardCondInt is the standard implementation of a Int and uses the
// sync/atomic package to manage a single int64 value.
type StandardCondInt struct {
	value    int64
	period   time.Duration
	lastSnap time.Time
}

// Snapshot returns a read-only copy of the Int.
func (g *StandardCondInt) Snapshot() CondInt {
	now := time.Now()
	if now.Sub(g.lastSnap) >= g.period {
		g.lastSnap = now
		return &CondIntSnapshot{g.Value(), true}
	}
	return &CondIntSnapshot{g.Value(), false}
}

// Writable return if the gauge should write to db current
func (g *StandardCondInt) Writable() bool {
	return time.Now().Sub(g.lastSnap) >= g.period
}

// Update updates the gauge's value.
func (g *StandardCondInt) Update(v int64) {
	atomic.StoreInt64(&g.value, v)
}

// Value returns the gauge's current value.
func (g *StandardCondInt) Value() int64 {
	return atomic.LoadInt64(&g.value)
}

//------------------------------------------------------------------------------

// CondFloat hold an int64 value that can be set arbitrarily.
type CondFloat interface {
	Snapshot() CondFloat
	Update(float64)
	Value() float64
	Writable() bool // 是否需要写入
}

// GetOrRegisterCondFloat returns an existing Int or constructs and registers a
// new StandardContInt.
func GetOrRegisterCondFloat(name string, r Registry, period time.Duration) CondFloat {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewCondFloat, period).(CondFloat)
}

// NewCondFloat constructs a new StandardCondFloat.
func NewCondFloat(period time.Duration) CondFloat {
	return &StandardCondFloat{
		value:    0.0,
		period:   period,
		lastSnap: time.Now(),
	}
}

// CondFloatSnapshot is a read-only copy of another Int.
type CondFloatSnapshot struct {
	value    float64
	writable bool
}

// Snapshot returns the snapshot.
func (g *CondFloatSnapshot) Snapshot() CondFloat { return g }

// Update panics.
func (g *CondFloatSnapshot) Update(float64) {
	panic("Update called on a IntSnapshot")
}

// Value returns the value at the time the snapshot was taken.
func (g *CondFloatSnapshot) Value() float64 { return float64(g.value) }

// Writable returns the value should write to db
func (g *CondFloatSnapshot) Writable() bool { return g.writable }

// StandardCondFloat is the standard implementation of a Int and uses the
// sync/atomic package to manage a single float64 value.
type StandardCondFloat struct {
	sync.Mutex
	value    float64
	period   time.Duration
	lastSnap time.Time
}

// Snapshot returns a read-only copy of the Int.
func (g *StandardCondFloat) Snapshot() CondFloat {
	now := time.Now()
	if now.Sub(g.lastSnap) >= g.period {
		g.lastSnap = now
		return &CondFloatSnapshot{g.Value(), true}
	}
	return &CondFloatSnapshot{g.Value(), false}
}

// Writable return if the gauge should write to db current
func (g *StandardCondFloat) Writable() bool {
	return time.Now().Sub(g.lastSnap) >= g.period
}

// Update updates the gauge's value.
func (g *StandardCondFloat) Update(v float64) {
	g.Lock()
	defer g.Unlock()
	g.value = v
}

// Value returns the gauge's current value.
func (g *StandardCondFloat) Value() float64 {
	g.Lock()
	defer g.Unlock()

	return g.value
}

// -----------------------------------------------------------------------------
// CondCounter
