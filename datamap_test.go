package metrics

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/guotie/assert"
)

func TestZDataMap(t *testing.T) {
	intHistoryFn := func(k string, p string) (fn func(DataMap) int64) {
		return func(g DataMap) int64 {
			vc := g.ValueInt64(k)
			vs5, ok := g.ValueHistory(k, p)
			if !ok {
				fmt.Printf("Not found history value: key=%s period=%s\n",
					k, p)
				return vc
			}
			//fmt.Printf("key=%s period=%s vc=%d vp=%v\n",
			//	k, p, vc, vs5)
			return vc - vs5.(int64)
		}
	}

	// ( value(k1, curr) - value(k1, p) ) / ( value(k2, curr) - value(k2, p) )
	rateFn := func(k1, k2, p string) func(DataMap) float64 {
		return func(g DataMap) float64 {
			k1c := g.ValueInt64(k1)
			k1p, ok := g.ValueHistory(k1, p)
			if !ok {
				k1p = int64(0)
			}

			k2c := g.ValueInt64(k2)
			k2p, ok := g.ValueHistory(k2, p)
			if !ok {
				k2p = int64(0)
			}

			return float64(k1c-k1p.(int64)) / float64(k2c-k2p.(int64))
		}
	}

	opt := &DataMapOption{
		Interval: time.Second * 2,
		Periods: map[string]time.Duration{
			"s5":  time.Second * 5,
			"s10": time.Second * 10,
			MS1:   M1,
		},

		DependentFuncs: map[string]interface{}{
			"pdponline-s5":    intHistoryFn("pdponline", "s5"),
			"pdptimeout-s5":   intHistoryFn("pdptimeout", "s5"),
			"timeout_rate-s5": rateFn("pdptimeout", "pdponline", "s5"),

			"pdponline-s10":    intHistoryFn("pdponline", "s10"),
			"pdptimeout-s10":   intHistoryFn("pdptimeout", "s10"),
			"timeout_rate-s10": rateFn("pdptimeout", "pdponline", "s10"),
		},

		KeyTypes: map[string]reflect.Type{
			"pdponline": reflect.TypeOf(&StandardCounter{}),
		},
		DepenentTypes: map[string]reflect.Type{
			// 5秒钟的变化量
			"pdponline-s5":    reflect.TypeOf(&StandardCounter{}),
			"pdptimeout-s5":   reflect.TypeOf(&StandardGauge{}),
			"timeout_rate-s5": reflect.TypeOf(&StandardGaugeFloat64{}),

			// 10秒变化量
			"pdponline-s10":    reflect.TypeOf(&StandardCounter{}),
			"pdptimeout-s10":   reflect.TypeOf(&StandardGauge{}),
			"timeout_rate-s10": reflect.TypeOf(&StandardGaugeFloat64{}),
		},
	}

	dm := GetOrRegisterDataMap("pdpdata", nil, opt)

	ms := dm.Snapshot(DefaultRegistry)
	assert.Assert(ms == nil, "Snapshot should be nil on first call")

	dm = GetOrRegisterDataMap("pdpdata", nil, opt)

	for i := 0; i < 40; i++ {
		dm.UpdateInt64("pdponline", int64(i*12))
		dm.UpdateInt64("pdptimeout", int64(i*4))

		ms = dm.Snapshot(DefaultRegistry)
		if i%2 != 0 || i == 0 {
			assert.Assert(ms == nil, "snapshot should be nil because of interval too short")
		} else {
			assert.Assert(ms != nil, "snapshot should not be nil")
			for _, m := range ms {
				printMeter(m)
			}
		}
		fmt.Println(time.Now(), "i =", i, "----------------------------------------------")
		time.Sleep(time.Second)
	}
}

// print meter
func printMeter(m interface{}) {

	switch m.(type) {
	case Counter:
		fmt.Printf("Counter: %d\n", m.(Counter).Count())
	case Gauge:
		fmt.Printf("Gauge: %d\n", m.(Gauge).Value())
	case GaugeFloat64:
		fmt.Printf("GaugeFloat64: %f\n", m.(GaugeFloat64).Value())
	}
}
