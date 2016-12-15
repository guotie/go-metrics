package metrics

import (
	"sync"
	"time"
)

// period counter是一个统计一段时间的总和和速率的计数器
// 例如, 统计5分钟，15分钟，30分钟，60分钟，1天的http请求总量和速率
//
// 注意: report 的间隔时间需要小于1分钟
//
const (
	// M1 1 minute
	M1 = "1m"
	// M5 5 minute
	M5 = "5m"
	// M15 15 minute
	M15 = "15m"
	// M30 30 minute
	M30 = "30m"
	// M60 60 minute
	M60 = "60m"
	// H1 60 minute, 1 hour
	H1 = "1h"
	// D1 1 day
	D1 = "1d"
)

var (
	m1  = time.Minute
	m5  = time.Minute * 5
	m15 = time.Minute * 15
	m30 = time.Minute * 30
	m60 = time.Hour
	h1  = time.Hour
	d1  = time.Hour * 24
)

// PeriodCounter Period Counter
type PeriodCounter interface {
	Clear()
	Inc(int64)
	Count() int64
	LatestPeriodCountRate(string) (int64, float64)

	Periods() []string
	SetProids(string, time.Duration)
}

// GetOrRegisterPeriodCounter returns an existing Counter or constructs and registers
// a new StandardCounter.
func GetOrRegisterPeriodCounter(name string, r Registry) PeriodCounter {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewPeriodCounter).(PeriodCounter)
}

// NewPeriodCounter constructs a new StandardPeriodCounter.
func NewPeriodCounter() PeriodCounter {
	if UseNilMetrics {
		return NilPeriodCounter{}
	}
	return &StandardPeriodCounter{
		periods:      make(map[string]time.Duration),
		latestCounts: make(map[string]int64),
		nextTs:       make(map[string]int64),
	}
}

// NewRegisteredPeriodCounter constructs and registers a new StandardPeriodCounter.
func NewRegisteredPeriodCounter(name string, r Registry) PeriodCounter {
	c := NewPeriodCounter()
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}

// NilPeriodCounter no-op PeriodCounter
type NilPeriodCounter struct{}

// Clear is a no-op.
func (NilPeriodCounter) Clear() {}

// Inc is a no-op.
func (NilPeriodCounter) Inc(int64) {}

// Count is a no-op.
func (NilPeriodCounter) Count() int64 { return 0 }

// LatestPeriodCountRate is a no-op.
func (NilPeriodCounter) LatestPeriodCountRate(string) (int64, float64) { return 0, 0.0 }

// Periods is a no-op.
func (NilPeriodCounter) Periods() []string { return []string{} }

// SetProids is a no-op.
func (NilPeriodCounter) SetProids(string, time.Duration) {}

// StandardPeriodCounter 默认 PeriodCounter 实现
type StandardPeriodCounter struct {
	sync.RWMutex
	count        int64
	periods      map[string]time.Duration
	latestCounts map[string]int64
	nextTs       map[string]int64 // period下次入库的timestamp(second)
}

// Clear clear count and latestCounts
func (pc *StandardPeriodCounter) Clear() {
	pc.Lock()
	defer pc.Unlock()

	pc.count = 0
	pc.latestCounts = map[string]int64{}
}

// Inc inc count
func (pc *StandardPeriodCounter) Inc(i int64) {
	pc.Lock()
	defer pc.Unlock()

	pc.count += i
}

// Count get count
func (pc *StandardPeriodCounter) Count() int64 {
	pc.RLock()
	defer pc.RUnlock()

	return pc.count
}

// LatestPeriodCountRate get latest period count rate
func (pc *StandardPeriodCounter) LatestPeriodCountRate(period string) (int64, float64) {
	pc.Lock()
	defer pc.Unlock()

	// period 不存在
	du, ok := pc.periods[period]
	if !ok {
		return 0, 0.0
	}

	// 判断当前时间戳是否满足入库条件
	ts := time.Now().Unix()
	nextTs := pc.nextTs[period]
	if ts < nextTs {
		return 0, 0.0
	}
	pc.nextTs[period] = nextTs + int64(du/time.Second)
	dcount := pc.count - pc.latestCounts[period]
	//println(period, pc.count, pc.latestCounts[period])

	// 更新该period的最近一次的值
	pc.latestCounts[period] = pc.count
	return dcount, float64(dcount) / float64(du/time.Second)
}

// Periods get periods of PeriodCounter
func (pc *StandardPeriodCounter) Periods() (periods []string) {
	pc.RLock()
	defer pc.RUnlock()

	for p := range pc.periods {
		periods = append(periods, p)
	}
	return
}

// SetProids set period
func (pc *StandardPeriodCounter) SetProids(p string, du time.Duration) {
	pc.Lock()
	defer pc.Unlock()

	if du == 0 {
		delete(pc.periods, p)
		delete(pc.nextTs, p)
	} else {
		// period是否已经存在
		if _, ok := pc.periods[p]; ok {
			return
		}

		pc.periods[p] = du
		nts := time.Now().Unix()
		mod := int64(60)
		// 设置下次汇报的时间戳
		// 如果是5分钟，15分钟，30分钟，60分钟，1天，设置为整点对齐
		switch du {
		case m5:
			mod = 300
		case m15:
			mod = 15 * 60
		case m30:
			mod = 30 * 60
		case m60:
			fallthrough
		case h1:
			mod = 3600
		case d1:
			mod = 86400
		default:
		}
		nts = nts - nts%mod + mod
		pc.nextTs[p] = nts
	}
}
