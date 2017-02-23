package metrics

import (
	"sync"
	"time"

	"github.com/guotie/days"
)

// period counter是一个统计一段时间的总和和速率的计数器
// 例如, 统计5分钟，15分钟，30分钟，60分钟，1天的http请求总量和速率
//
// 注意: report 的间隔时间需要小于1分钟
//
const (
	// MS1 1 minute
	MS1 = "1m"
	// MS5 5 minute
	MS5 = "5m"
	// MS15 15 minute
	MS15 = "15m"
	// MS30 30 minute
	MS30 = "30m"

	// H1 60 minute, 1 hour
	HS1 = "1h"
	// D1 1 day
	DS1 = "1d"
)

var (
	M1  = time.Minute
	M5  = time.Minute * 5
	M15 = time.Minute * 15
	M30 = time.Minute * 30
	H1  = time.Hour
	D1  = time.Hour * 24
)

// PeriodCounter Period Counter
type PeriodCounter interface {
	Clear()
	Inc(int64)
	Count() int64
	LatestPeriodCountRate(string) (int64, float64)

	Periods() []string
	SetPeriod(string, time.Duration)
	SetPeriods(map[string]time.Duration)
	Snapshot() PeriodCounter
	Writable() bool
}

// GetOrRegisterPeriodCounter returns an existing Counter or constructs and registers
// a new StandardCounter.
// cb should be type of map[string]time.Duration
func GetOrRegisterPeriodCounter(name string, r Registry, cb interface{}) PeriodCounter {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewPeriodCounter, cb).(PeriodCounter)
}

// NewPeriodCounter constructs a new StandardPeriodCounter.
// cb should be type of map[string]time.Duration
func NewPeriodCounter(cb interface{}) PeriodCounter {
	pc := &StandardPeriodCounter{
		periods:      make(map[string]time.Duration),
		latestCounts: make(map[string]int64),
		nextTs:       make(map[string]int64),
		lastSnap:     time.Now(),
	}
	if cb != nil {
		pc.SetPeriods(cb.(map[string]time.Duration))
	}

	return pc
}

// NewRegisteredPeriodCounter constructs and registers a new StandardPeriodCounter.
// cb is period
func NewRegisteredPeriodCounter(name string, r Registry, cb interface{}) PeriodCounter {
	c := NewPeriodCounter(cb)
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}

// countRate count and rate
type countRate struct {
	count int64
	rate  float64
}

// PeriodCounterSnapshot is a read-only copy of another PeriodCounter.
type PeriodCounterSnapshot struct {
	count        int64
	writable     bool // 是否可以入库
	periodCounts map[string]countRate
}

// Clear panics.
func (*PeriodCounterSnapshot) Clear() { panic("Clear called on a PeriodCounterSnapshot") }

// Inc panics.
func (*PeriodCounterSnapshot) Inc(int64) { panic("Inc called on a PeriodCounterSnapshot") }

// SetPeriod panics.
func (*PeriodCounterSnapshot) SetPeriod(string, time.Duration) {
	panic("SetPeriod called on a PeriodCounterSnapshot")
}

// SetPeriods panics.
func (*PeriodCounterSnapshot) SetPeriods(map[string]time.Duration) {
	panic("SetPeriods called on a PeriodCounterSnapshot")
}

// Count return count
func (pcs *PeriodCounterSnapshot) Count() int64 { return pcs.count }

// Writable return should insert to db
func (pcs *PeriodCounterSnapshot) Writable() bool { return pcs.writable }

// LatestPeriodCountRate return period count and rate of the period
func (pcs *PeriodCounterSnapshot) LatestPeriodCountRate(period string) (int64, float64) {
	return pcs.periodCounts[period].count, pcs.periodCounts[period].rate
}

// Periods return periods of snapshot
func (pcs *PeriodCounterSnapshot) Periods() []string {
	ps := make([]string, 0, len(pcs.periodCounts))
	for p := range pcs.periodCounts {
		ps = append(ps, p)
	}
	return ps
}

// Snapshot returns the snapshot.
func (pcs *PeriodCounterSnapshot) Snapshot() PeriodCounter { return pcs }

// StandardPeriodCounter 默认 PeriodCounter 实现
type StandardPeriodCounter struct {
	sync.RWMutex
	count        int64
	periods      map[string]time.Duration
	latestCounts map[string]int64
	nextTs       map[string]int64 // period下次入库的timestamp(second)
	lastSnap     time.Time
	minPeriod    time.Duration
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

	ts := time.Now().Unix()
	return pc.getPeriodCountRate(period, ts)
}

func (pc *StandardPeriodCounter) getPeriodCountRate(period string, ts int64) (int64, float64) {
	// period 不存在
	du, ok := pc.periods[period]
	if !ok {
		return -1, -1.0
	}

	// 判断当前时间戳是否满足入库条件
	nextTs := pc.nextTs[period]
	if ts < nextTs {
		return -1, -1.0
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

// SetPeriod set period
func (pc *StandardPeriodCounter) SetPeriod(p string, du time.Duration) {
	pc.Lock()
	defer pc.Unlock()

	if pc.minPeriod == 0 {
		pc.minPeriod = du
	} else if pc.minPeriod > du {
		pc.minPeriod = du
	}

	pc.setPeriod(p, du, time.Now())
}

// SetPeriods set periods
func (pc *StandardPeriodCounter) SetPeriods(ps map[string]time.Duration) {
	pc.Lock()
	defer pc.Unlock()

	ts := time.Now()
	for p, du := range ps {
		pc.setPeriod(p, du, ts)
	}
}

// setPeriod set period, lock before called
func (pc *StandardPeriodCounter) setPeriod(p string, du time.Duration, tm time.Time) {
	if du == 0 {
		delete(pc.periods, p)
		delete(pc.nextTs, p)
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
	}
	// 间隔时间为天时, 需修正时区
	if du == D1 {
		nts = days.Tomorrow(tm).Unix()
	} else {
		nts = nts - nts%mod + mod
	}
	pc.nextTs[p] = nts
}

// Writable return should insert to db
func (pc *StandardPeriodCounter) Writable() bool {
	now := time.Now()
	return now.Sub(pc.lastSnap) >= pc.minPeriod
}

// Snapshot snapshot of StandardPeriodCounter
func (pc *StandardPeriodCounter) Snapshot() PeriodCounter {
	pc.Lock()
	defer pc.Unlock()

	now := time.Now()
	ts := now.Unix()
	if now.Sub(pc.lastSnap) < pc.minPeriod {
		return &PeriodCounterSnapshot{
			writable: false,
			count:    0,
		}
	}

	// 更新lastSnap
	pc.lastSnap = now
	pcs := &PeriodCounterSnapshot{
		writable:     true,
		count:        pc.count,
		periodCounts: make(map[string]countRate),
	}

	for p := range pc.periods {
		count, rate := pc.getPeriodCountRate(p, ts)
		pcs.periodCounts[p] = countRate{count, rate}
	}

	return pcs
}
