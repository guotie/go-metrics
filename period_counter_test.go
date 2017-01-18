package metrics

import (
	"fmt"
	"testing"
	"time"
)

func testPeriodCounter(t *testing.T) {
	c := GetOrRegisterPeriodCounter("period_counter", nil)
	c.SetPeriod(MS1, M1)
	c.SetPeriod(MS5, M5)

	c.Inc(10)
	if c.Count() != 10 {
		t.Fail()
	}

	c.Inc(20)
	if c.Count() != 30 {
		t.Fail()
	}

	ps := c.Periods()
	if len(ps) != 2 && (ps[0] != MS1 && ps[0] != MS5) {
		t.Fail()
	}

	pc, ok := c.(*StandardPeriodCounter)
	if !ok {
		t.Fail()
		return
	}

	now := time.Now().Unix()
	ts := pc.nextTs[MS1]
	fmt.Printf("report timestamp is %v, will sleep %v second\n",
		time.Unix(ts, 0), ts-now)
	time.Sleep(time.Second * time.Duration(ts-now))

	total, rate := pc.LatestPeriodCountRate(MS1)
	if total != 30 {
		t.Fail()
	}
	fmt.Printf("total=%v rate=%v\n", total, rate)
	// 第二次取数据，返回0
	total, rate = pc.LatestPeriodCountRate(MS1)
	if total != 0 || rate != 0.0 {
		t.Fail()
	}
	//
	pc.Inc(120)
	time.Sleep(pc.periods[MS1])

	// 第二次取数据，返回0
	total, rate = pc.LatestPeriodCountRate(MS1)
	if total != 120 || rate != 2.0 {
		t.Fail()
	}
}
