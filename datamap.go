package metrics

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/guotie/days"
)

// DataMap hold an int64 value that can be set arbitrarily.
// DataMap保存一组数据, 和这组数据的历史数据, 并设置可以计算数据关系的函数来计算因变量的值
type DataMap interface {
	Snapshot(r Registry) []interface{}

	UpdateInt64(string, int64)     // 设置自变量的值 int64
	UpdateFloat64(string, float64) // 设置自变量的值 float64

	// Value函数无锁
	Value(string) interface{}    // 自变量的值
	ValueInt64(string) int64     // 自变量的值int64
	ValueFloat64(string) float64 // 自变量的值float64
	ValueHistory(string, string) (interface{}, bool)

	// DependentValue(string) interface{}    // 因变量的值
	// DependentValueInt64(string) int64     // 因变量的值int64
	// DependentValueFloat64(string) float64 // 因变量的值float64

	Periods() []string       // 保存那些历史数据
	Keys() []string          // 自变量列表
	DependentKeys() []string // 因变量列表

	SetPeriods(map[string]time.Duration)   // 历史数据
	SetKeyType(string, reflect.Type, bool) // 设置 key type
	SetDependentFuncs(string, interface{}) // 因变量
}

// DataMapOption datamap options
// option of DataMap
type DataMapOption struct {
	Prefix         string
	Interval       time.Duration
	Periods        map[string]time.Duration
	DependentFuncs map[string]interface{}
	KeyTypes       map[string]reflect.Type
	DepenentTypes  map[string]reflect.Type
}

// 各种meter type
var (
	counterType      = reflect.TypeOf(&StandardCounter{0})
	gaugeType        = reflect.TypeOf(&StandardGauge{0})
	gaugeFloat64Type = reflect.TypeOf(&StandardGaugeFloat64{})
	histogramType    = reflect.TypeOf(&StandardHistogram{})
	meterType        = reflect.TypeOf(&StandardMeter{})
	timerType        = reflect.TypeOf(&StandardTimer{})
)

// GetOrRegisterDataMap returns an existing datamap or constructs and registers a
// new StandardDataMap.
func GetOrRegisterDataMap(name string, r Registry, opt *DataMapOption) DataMap {
	if nil == r {
		r = DefaultRegistry
	}

	return r.GetOrRegister(name, NewDataMap, opt).(DataMap)
}

// NewDataMap constructs a new StandardDataMap.
func NewDataMap(prefix string, opt *DataMapOption) DataMap {
	gm := &StandardDataMap{
		minInterval:    time.Second * 60,
		latestSnapshot: time.Now(),
		prefix:         prefix,
		values:         make(map[string]interface{}),
		valuesHistory:  make(map[string]map[string]interface{}),
		dependentFuncs: make(map[string]interface{}),
		keyTypes:       make(map[string]reflect.Type),
		dependentTypes: make(map[string]reflect.Type),
		periods:        make(map[string]time.Duration),
		nextTs:         make(map[string]int64),
	}

	if opt == nil {
		panic("NewDataMap: param opt should NOT be nil")
	}

	if opt.Interval != 0 {
		// 设置 minInterval
		gm.minInterval = opt.Interval
	}

	gm.SetPeriods(opt.Periods)
	for k, v := range opt.DependentFuncs {
		gm.SetDependentFuncs(k, v)
	}

	// key types
	for k, t := range opt.KeyTypes {
		gm.SetKeyType(k, t, false)
	}

	// dependent key types
	for k, t := range opt.DepenentTypes {
		gm.SetKeyType(k, t, true)
	}

	return gm
}

// NewRegisteredDataMap constructs and registers a new StandardDataMap.
func NewRegisteredDataMap(name string, r Registry, opt *DataMapOption) DataMap {
	c := NewDataMap(name, opt)
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}

// FloatValueFunc func which return float64
type FloatValueFunc func(DataMap) float64

// IntValueFunc func return int64
type IntValueFunc func(DataMap) int64

// StandardDataMap is the standard implementation of a Gauge and uses the
// sync/atomic package to manage a single int64 value.
type StandardDataMap struct {
	sync.RWMutex

	minInterval    time.Duration // 最小间隔
	latestSnapshot time.Time

	prefix string // 加在产生的meter前作为前缀

	values         map[string]interface{}            // 当前值
	valuesHistory  map[string]map[string]interface{} // 历史值
	dependentFuncs map[string]interface{}            // 计算因变量的值

	keyTypes       map[string]reflect.Type
	dependentTypes map[string]reflect.Type

	periods map[string]time.Duration
	nextTs  map[string]int64 // period下次入库的timestamp(second)
}

// Prefix prefix of datamap
func (g *StandardDataMap) Prefix() string {
	return g.prefix
}

// snapshotable return whether snapshot
func (g *StandardDataMap) snapshotable() bool {
	tm := time.Now()
	if tm.Sub(g.latestSnapshot) > g.minInterval {
		g.latestSnapshot = tm
		return true
	}

	return false
}

// Snapshot returns a read-only copy of the gauge.
// 根据key的类型，输入相应的 meter
// 同时, snapshot 需要判断是否需要计算历史数据
func (g *StandardDataMap) Snapshot(r Registry) []interface{} {
	g.Lock()
	defer g.Unlock()

	if g.snapshotable() == false {
		return nil
	}

	//fmt.Printf("Snap shot data map keyTypes: %d dependentKeyTypes: %d ....\n",
	//	len(g.keyTypes), len(g.dependentTypes))
	meters := make([]interface{}, 0)
	for k, t := range g.keyTypes {
		// 自变量
		val, ok := g.values[k]
		if !ok {
			continue
		}
		//fmt.Printf("independent typ: %v\n", k)
		meters = append(meters, g.generateMeter(k, val, false, r, t))
	}

	for k, t := range g.dependentTypes {
		// 因变量
		val, ok := g.dependentFuncs[k]
		if !ok {
			continue
		}
		//fmt.Printf("dependent typ: %v\n", k)
		meters = append(meters, g.generateMeter(k, val, true, r, t))
	}

	// 更新历史数据
	g.updateHistory()
	return meters
}

// 生成响应的 meter
func (g *StandardDataMap) generateMeter(key string, val interface{},
	dependent bool, r Registry, typ reflect.Type) interface{} {
	if dependent {
		// 计算val的值
		switch val.(type) {
		case IntValueFunc:
			fn := val.(IntValueFunc)
			val = fn(g)

		case func(DataMap) int64:
			fn := val.(func(DataMap) int64)
			val = fn(g)

		case FloatValueFunc:
			fn := val.(FloatValueFunc)
			val = fn(g)

		case func(DataMap) float64:
			fn := val.(func(DataMap) float64)
			val = fn(g)

		default:
			panic(fmt.Sprintf("invalid func type: key=%s func typ: %v",
				key, reflect.TypeOf(val)))
		}
	}

	switch typ {
	case counterType:
		c := GetOrRegisterCounter(g.prefix+"-"+key, r)
		c.Inc(val.(int64))
		return c

	case gaugeType:
		m := GetOrRegisterGauge(g.prefix+"-"+key, r)
		m.Update(val.(int64))
		return m

	case gaugeFloat64Type:
		m := GetOrRegisterGaugeFloat64(g.prefix+"-"+key, r)
		m.Update(val.(float64))
		return m

	case histogramType:
		panic("Not support Histogram type")

	case meterType:
		m := GetOrRegisterMeter(g.prefix+"-"+key, r)
		m.Mark(val.(int64))
		return m

	case timerType:
		panic("Not support timer meter type")
	}

	return nil
}

// updateHistory 更新历史数据
// caller lock
func (g *StandardDataMap) updateHistory() {
	ts := time.Now().Unix()

	for p, nts := range g.nextTs {
		if ts >= nts {
			du, ok := g.periods[p]
			if !ok {
				panic("Not found period " + p)
			}

			//fmt.Printf("update period %s\n", p)
			// update history values
			his, ok := g.valuesHistory[p]
			if !ok {
				g.valuesHistory[p] = make(map[string]interface{})
				his = g.valuesHistory[p]
			}

			for k, v := range g.values {
				his[k] = v
			}

			g.nextTs[p] += int64(du / time.Second)
		}
	}
}

// Periods return periods
func (g *StandardDataMap) Periods() []string {
	g.Lock()
	defer g.Unlock()

	ps := make([]string, len(g.periods))
	i := 0
	for k := range g.periods {
		ps[i] = k
	}
	return ps
}

// SetPeriods set periods
func (g *StandardDataMap) SetPeriods(p map[string]time.Duration) {
	g.Lock()
	defer g.Unlock()

	ts := time.Now()
	for s, t := range p {
		g.setPeriod(s, t, ts)
	}
}

// setPeriod set period, lock before called
func (pc *StandardDataMap) setPeriod(p string, du time.Duration, tm time.Time) {
	if du == 0 {
		panic("setPeriod: invalid duration: 0")
		return
	}
	// period是否已经存在
	if _, ok := pc.periods[p]; ok {
		return
	}

	nts := tm.Unix()
	pc.periods[p] = du
	mod := int64(60)
	// 设置下次汇报的时间戳
	// 如果是5分钟，15分钟，30分钟，60分钟，1天，设置为整点对齐
	switch du {
	case M5:
		mod = 300
	case M15:
		mod = 15 * 60
	case M30:
		mod = 30 * 60

	case H1:
		mod = 3600
	case D1:
		mod = 86400
	default:
		// 小于1分钟
		if du < M1 {
			mod = 1
		}
	}

	// 间隔时间为天时, 需修正时区
	if du == D1 {
		nts = days.Tomorrow(tm).Unix()
	} else {
		nts = nts - nts%mod + mod
	}
	pc.nextTs[p] = nts
}

// SetKeyType 设置 key type
func (g *StandardDataMap) SetKeyType(key string, ty reflect.Type, isDependent bool) {
	g.Lock()
	defer g.Unlock()

	if isDependent == false {
		// 因变量
		// dependent variables
		g.keyTypes[key] = ty
	} else {
		// 自变量
		// independent variables
		g.dependentTypes[key] = ty
	}
}

// UpdateInt64 updates the gauge's value.
func (g *StandardDataMap) UpdateInt64(key string, v int64) {
	g.Lock()
	defer g.Unlock()

	g.values[key] = v
	//fmt.Println(key, "prev=", g.valuePrev[key], g.value[key])
}

// UpdateFloat64 updates the gauge's value.
func (g *StandardDataMap) UpdateFloat64(key string, v float64) {
	g.Lock()
	defer g.Unlock()

	g.values[key] = v
}

// Value returns the gauge's current value.
// caller should lock
func (g *StandardDataMap) Value(key string) interface{} {
	return g.values[key]
}

// ValueInt64 get int64 value
// caller should lock
func (g *StandardDataMap) ValueInt64(key string) int64 {
	v, ok := g.values[key]
	if !ok {
		return 0
	}
	return v.(int64)
}

// ValueFloat64 return the gauge's float64 value of key.
// caller should lock
func (g *StandardDataMap) ValueFloat64(key string) float64 {
	vf, ok := g.values[key]
	if !ok {
		return 0.0
	}
	return vf.(float64)
}

// ValueHistory 历史值
// caller should lock
func (g *StandardDataMap) ValueHistory(key, period string) (interface{}, bool) {
	if his, ok := g.valuesHistory[period]; ok {
		return his[key], true
	}

	return nil, false
}

// Keys return keys
func (g *StandardDataMap) Keys() []string {
	g.Lock()
	g.Unlock()

	keys := make([]string, len(g.values))
	i := 0
	for k := range g.values {
		keys[i] = k
	}
	return keys
}

// DependentKeys return keys
func (g *StandardDataMap) DependentKeys() []string {
	g.RLock()
	defer g.RUnlock()

	keys := make([]string, len(g.dependentFuncs))
	i := 0
	for k := range g.dependentFuncs {
		keys[i] = k
	}
	return keys
}

// SetDependentFuncs set IntValueFunc
func (g *StandardDataMap) SetDependentFuncs(key string, fn interface{}) {
	g.Lock()
	defer g.Unlock()

	switch fn.(type) {
	case IntValueFunc:
		g.dependentFuncs[key] = fn

	case func(DataMap) int64:
		g.dependentFuncs[key] = IntValueFunc(fn.(func(DataMap) int64))

	case FloatValueFunc:
		g.dependentFuncs[key] = fn

	case func(DataMap) float64:
		g.dependentFuncs[key] = FloatValueFunc(fn.(func(DataMap) float64))

	default:
		panic(fmt.Sprintf("invalid type of param fn, should be IntValueFunc or FloatValueFunc: %v",
			reflect.TypeOf(fn)))
	}

	return
}
