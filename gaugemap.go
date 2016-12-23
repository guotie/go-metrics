package metrics

import (
	"fmt"
	"reflect"
	"sync"
)

// GaugeMap hold an int64 value that can be set arbitrarily.
type GaugeMap interface {
	Snapshot() GaugeMap
	UpdateInt64(string, int64)
	UpdateFloat64(string, float64)
	Value(string) interface{}
	ValueInt64(string) int64
	ValueFloat64(string) float64
	Keys() []string
	SetFuncKey(string, interface{}) error
}

// GetOrRegisterGaugeMap returns an existing Gauge or constructs and registers a
// new StandardGauge.
func GetOrRegisterGaugeMap(name string, r Registry) GaugeMap {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewGaugeMap).(GaugeMap)
}

// NewGaugeMap constructs a new StandardGaugeMap.
func NewGaugeMap() GaugeMap {
	if UseNilMetrics {
		return NilGaugeMap{}
	}
	return &StandardGaugeMap{value: make(map[string]interface{}),
		valuePrev: make(map[string]interface{}),
	}
}

// NewRegisteredGaugeMap constructs and registers a new StandardGaugeMap.
func NewRegisteredGaugeMap(name string, r Registry) GaugeMap {
	c := NewGaugeMap()
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}

// GaugeMapSnapshot is a read-only copy of another Gauge.
type GaugeMapSnapshot struct {
	i map[string]int64
	f map[string]float64
}

// Snapshot returns the snapshot.
func (g *GaugeMapSnapshot) Snapshot() GaugeMap { return g }

// UpdateInt64 panics.
func (*GaugeMapSnapshot) UpdateInt64(string, int64) {
	panic("UpdateInt64 called on a GaugeMapSnapshot")
}

// UpdateFloat64 panics.
func (*GaugeMapSnapshot) UpdateFloat64(string, float64) {
	panic("UpdateFloat64 called on a GaugeMapSnapshot")
}

// Value returns the value at the time the snapshot was taken.
func (g *GaugeMapSnapshot) Value(key string) interface{} {
	if val, ok := g.i[key]; ok {
		return val
	}
	if val, ok := g.f[key]; ok {
		return val
	}
	return nil
}

// ValueInt64 return the value and convert it to int64
func (g *GaugeMapSnapshot) ValueInt64(key string) int64 {
	if val, ok := g.i[key]; ok {
		return val
	}
	panic(fmt.Sprintf("Not found key %s in int map", key))
}

// ValueFloat64 return the value and convert it to float64
func (g *GaugeMapSnapshot) ValueFloat64(key string) float64 {
	if val, ok := g.f[key]; ok {
		return val
	}
	panic(fmt.Sprintf("Not found key %s in float map", key))
}

// Keys return keys
func (g *GaugeMapSnapshot) Keys() (keys []string) {
	for k := range g.i {
		keys = append(keys, k)
	}
	for k := range g.f {
		keys = append(keys, k)
	}
	return keys
}

// SetFuncKey return keys
func (g *GaugeMapSnapshot) SetFuncKey(key string, fn interface{}) error {
	panic("SetFuncKey called on GaugeMapSnapshot")
}

// NilGaugeMap is a no-op Gauge.
type NilGaugeMap struct{}

// Snapshot is a no-op.
func (NilGaugeMap) Snapshot() GaugeMap { return NilGaugeMap{} }

// UpdateInt64 is a no-op.
func (NilGaugeMap) UpdateInt64(string, int64) {}

// UpdateFloat64 is a no-op.
func (NilGaugeMap) UpdateFloat64(string, float64) {}

// Value is a no-op.
func (NilGaugeMap) Value(string) interface{} { return 0 }

// ValueInt64 is a no-op.
func (NilGaugeMap) ValueInt64(string) int64 { return 0 }

// ValueFloat64 is a no-op.
func (NilGaugeMap) ValueFloat64(string) float64 { return 0 }

// Keys return keys
func (NilGaugeMap) Keys() []string {
	return []string{}
}

// SetFuncKey return keys
func (NilGaugeMap) SetFuncKey(string, interface{}) error { return nil }

// FloatValueFunc func which return float64
type FloatValueFunc func(GaugeMap) float64

// IntValueFunc func return int64
type IntValueFunc func(GaugeMap) int64

// StandardGaugeMap is the standard implementation of a Gauge and uses the
// sync/atomic package to manage a single int64 value.
type StandardGaugeMap struct {
	sync.RWMutex
	value     map[string]interface{} // int64, float64, or function values
	valuePrev map[string]interface{} // 计算时有可能会应用到之前的数据
	keys      []string
}

// Snapshot returns a read-only copy of the gauge.
func (g *StandardGaugeMap) Snapshot() GaugeMap {
	g.Lock()
	defer g.Unlock()

	snap := &GaugeMapSnapshot{
		i: map[string]int64{},
		f: map[string]float64{},
	}

	for key, v := range g.value {
		switch v.(type) {
		case int64:
			snap.i[key] = v.(int64)

		case float64:
			snap.f[key] = v.(float64)

		case FloatValueFunc:
			ff := v.(FloatValueFunc)
			fv := ff(g)
			snap.f[key] = fv

		case IntValueFunc:
			fi := v.(IntValueFunc)
			iv := fi(g)
			snap.i[key] = iv

		default:
			panic("invalid value type of GaugeMap")
		}
	}

	return snap
}

// UpdateInt64 updates the gauge's value.
func (g *StandardGaugeMap) UpdateInt64(key string, v int64) {
	g.Lock()
	defer g.Unlock()

	if pv, ok := g.value[key]; ok {
		g.valuePrev[key] = pv.(int64)
	} else {
		g.valuePrev[key] = int64(0)
	}

	g.value[key] = v
	//fmt.Println(key, "prev=", g.valuePrev[key], g.value[key])
}

// UpdateFloat64 updates the gauge's value.
func (g *StandardGaugeMap) UpdateFloat64(key string, v float64) {
	g.Lock()
	defer g.Unlock()

	if pv, ok := g.value[key]; ok {
		g.valuePrev[key] = pv.(float64)
	} else {
		g.valuePrev[key] = float64(0.0)
	}

	g.value[key] = v
}

// Value returns the gauge's current value.
func (g *StandardGaugeMap) Value(key string) interface{} {
	g.RLock()
	defer g.RUnlock()

	return g.value[key]
}

// ValueInt64 get int64 value
func (g *StandardGaugeMap) ValueInt64(key string) int64 {
	g.RLock()
	defer g.RUnlock()

	var vi int64
	v := g.value[key]
	switch v.(type) {
	case int64:
		vi = v.(int64)
	case IntValueFunc:
		vi = v.(IntValueFunc)(g)
	default:
		panic("invalid value type of key " + key)
	}
	return vi
}

// ValueFloat64 return the gauge's float64 value of key.
func (g *StandardGaugeMap) ValueFloat64(key string) float64 {
	g.RLock()
	defer g.RUnlock()

	var vf float64
	v := g.value[key]
	switch v.(type) {
	case float64:
		vf = v.(float64)
	case FloatValueFunc:
		vf = v.(FloatValueFunc)(g)
	default:
		panic("invalid value type of key " + key)
	}
	return vf
}

// Keys return keys
func (g *StandardGaugeMap) Keys() []string {
	return g.keys
}

// SetFuncKey set IntValueFunc
func (g *StandardGaugeMap) SetFuncKey(key string, fn interface{}) error {
	g.Lock()
	defer g.Unlock()

	switch fn.(type) {
	case IntValueFunc:
		g.value[key] = fn
	case func(GaugeMap) int64:
		g.value[key] = IntValueFunc(fn.(func(GaugeMap) int64))
	case FloatValueFunc:
		g.value[key] = fn
	case func(GaugeMap) float64:
		g.value[key] = FloatValueFunc(fn.(func(GaugeMap) float64))

	default:
		panic(fmt.Sprintf("invalid type of param fn, should be IntValueFunc or FloatValueFunc: %v",
			reflect.TypeOf(fn)))
	}

	return nil
}
