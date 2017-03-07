go-metrics
==========

![travis build status](https://travis-ci.org/rcrowley/go-metrics.svg?branch=master)

Go port of Coda Hale's Metrics library: <https://github.com/dropwizard/metrics>.

Documentation: <http://godoc.org/github.com/rcrowley/go-metrics>.

Usage
-----

Create and update metrics:

```go
c := metrics.NewCounter()
metrics.Register("foo", c)
c.Inc(47)

g := metrics.NewGauge()
metrics.Register("bar", g)
g.Update(47)

r := NewRegistry()
g := metrics.NewRegisteredFunctionalGauge("cache-evictions", r, func() int64 { return cache.getEvictionsCount() })

s := metrics.NewExpDecaySample(1028, 0.015) // or metrics.NewUniformSample(1028)
h := metrics.NewHistogram(s)
metrics.Register("baz", h)
h.Update(47)

m := metrics.NewMeter()
metrics.Register("quux", m)
m.Mark(47)

t := metrics.NewTimer()
metrics.Register("bang", t)
t.Time(func() {})
t.Update(47)
```

Register() is not threadsafe. For threadsafe metric registration use
GetOrRegister:

```
t := metrics.GetOrRegisterTimer("account.create.latency", nil)
t.Time(func() {})
t.Update(47)
```

Add Meter Types:

增加了两个类型，这两个类型的Register都需要传入参数。同时，写入influxdb时，需要配合使用 github.com/guotie/go-metrics-influxdb

## DataMap

DataMap用来记录一系列值的关系和关系的历史变化情况, 并能设置入库间隔时间。

DataMap使用方法如下：
1. 注册或者获取DataMap，注册时，需要传入一个参数 DataMapOption，定义如下：

```go
type DataMapOption struct {
	Prefix        string
	Interval      time.Duration
	Periods       map[string]time.Duration
	KeyTypes      map[string]reflect.Type
	KeyPeriod     time.Duration // 自变量入库间隔
	DependentVars map[string]*DependentVar
}
```

其中：
- Prefix：   DataMap的名字
- Interval： DataMap的入库间隔
- Periods：  DataMap记录的历史值间隔
- keyTyps:   自变量列表
- KeyPeriod: 自变量入库间隔
- DependentVars: 所有需要通过自变量来计算得到的因变量的列表

DependentVar定义如下：

```go
// DependentVar depend var
type DependentVar struct {
	Name     string
	Func     interface{} // 计算规则
	Typ      reflect.Type
	Period   time.Duration
	lastSnap time.Time // 上次snapshot时间
}
```

其中：
- Name：  因变量的名字
- Func:   计算该因变量的函数
- Typ：   因变量的类型
- Period：因变量的入库间隔
- lastSnap: 上次snapshot时间

用法示例如下：

```go

var (
	minute  = time.Minute
	periods = map[string]time.Duration{
		metrics.MS1:  metrics.M1,
		metrics.MS5:  metrics.M5,
		metrics.MS15: metrics.M15,
		metrics.MS30: metrics.M30,
		metrics.HS1:  metrics.H1,
		metrics.DS1:  metrics.D1,
	}

	gaugeType      = reflect.TypeOf(&metrics.StandardCondInt{})
	gaugeFloatType = reflect.TypeOf(&metrics.StandardCondFloat{})

	//  value(k1, curr) / value(k2, curr)
	// ( value(k1, curr) - value(k1, period) ) / ( value(k2, curr) - value(k2, period) )
	relRateFn = func(k1, k2, period string) func(metrics.DataMap) float64 {
		return func(g metrics.DataMap) float64 {
			if period == "" {
				return float64(g.ValueInt64(k1)) / float64(g.ValueInt64(k2))
			}
			// k1当前值
			k1c := g.ValueInt64(k1)
			// k1历史值
			k1p, ok := g.ValueHistory(k1, period)
			if !ok {
				k1p = int64(0)
				return 0.0
			}

			// k2当前值
			k2c := g.ValueInt64(k2)
			// k2历史值
			k2p, ok := g.ValueHistory(k2, period)
			if !ok {
				k2p = int64(0)
				return 0.0
			}

			return float64(k1c-k1p.(int64)) / float64(k2c-k2p.(int64))
		}
	}
	// 返回relRateFn绝对值
	absRelRateFn = func(k1, k2, period string) func(metrics.DataMap) float64 {
		return func(g metrics.DataMap) float64 {
			if period == "" {
				return float64(g.ValueInt64(k1)) / float64(g.ValueInt64(k2))
			}
			// k1当前值
			k1c := g.ValueInt64(k1)
			// k1历史值
			k1p, ok := g.ValueHistory(k1, period)
			if !ok {
				k1p = int64(0)
				return 0.0
			}

			// k2当前值
			k2c := g.ValueInt64(k2)
			// k2历史值
			k2p, ok := g.ValueHistory(k2, period)
			if !ok {
				k2p = int64(0)
				return 0.0
			}

			ret := float64(k1c-k1p.(int64)) / float64(k2c-k2p.(int64))
			if ret < 0 {
				return 0.0
			}
			return ret
		}
	}

	waitTmoRate = func(period string) func(metrics.DataMap) float64 {
		return relRateFn("wait_timeout", "pdpc_total", period)
	}
	updateTmoRate = func(period string) func(metrics.DataMap) float64 {
		return relRateFn("update_waiting_timeout", "update_total", period)
	}

	pdpOpt = &metrics.DataMapOption{
		Interval: minute,
		Periods:  periods,
		DependentVars: map[string]*metrics.DependentVar{
			"wait_tmo_rate": &metrics.DependentVar{
				Func:   waitTmoRate(""),
				Typ:    gaugeFloatType,
				Period: metrics.M1,
			},
			"wait_tmo_rate_1m": &metrics.DependentVar{
				Func:   waitTmoRate(metrics.MS1),
				Typ:    gaugeFloatType,
				Period: metrics.M1,
			},
			"wait_tmo_rate_5m": &metrics.DependentVar{
				Func:   waitTmoRate(metrics.MS5),
				Typ:    gaugeFloatType,
				Period: metrics.M5,
			},
			"wait_tmo_rate_15m": &metrics.DependentVar{
				Func:   waitTmoRate(metrics.MS15),
				Typ:    gaugeFloatType,
				Period: metrics.M15,
			},
			"wait_tmo_rate_30m": &metrics.DependentVar{
				Func:   waitTmoRate(metrics.MS30),
				Typ:    gaugeFloatType,
				Period: metrics.M30,
			},
			"wait_tmo_rate_1h": &metrics.DependentVar{
				Func:   waitTmoRate(metrics.HS1),
				Typ:    gaugeFloatType,
				Period: metrics.H1,
			},
			"wait_tmo_rate_1d": &metrics.DependentVar{
				Func:   waitTmoRate(metrics.DS1),
				Typ:    gaugeFloatType,
				Period: metrics.D1,
			},

			"update_tmo_rate_1m": &metrics.DependentVar{
				Func:   updateTmoRate(metrics.MS1),
				Typ:    gaugeFloatType,
				Period: metrics.M1,
			},
			"update_tmo_rate_5m": &metrics.DependentVar{
				Func:   updateTmoRate(metrics.MS5),
				Typ:    gaugeFloatType,
				Period: metrics.M5,
			},
			"update_tmo_rate_15m": &metrics.DependentVar{
				Func:   updateTmoRate(metrics.MS15),
				Typ:    gaugeFloatType,
				Period: metrics.M15,
			},
			"update_tmo_rate_30m": &metrics.DependentVar{
				Func:   updateTmoRate(metrics.MS30),
				Typ:    gaugeFloatType,
				Period: metrics.M30,
			},
			"update_tmo_rate_1h": &metrics.DependentVar{
				Func:   updateTmoRate(metrics.HS1),
				Typ:    gaugeFloatType,
				Period: metrics.H1,
			},
			"update_tmo_rate_1d": &metrics.DependentVar{
				Func:   updateTmoRate(metrics.DS1),
				Typ:    gaugeFloatType,
				Period: metrics.D1,
			},
		},

		KeyTypes: map[string]reflect.Type{
			"pdpc_online":    gaugeType,
			"wait_list":      gaugeType,
			"update_waiting": gaugeType,
			"pdpc_failed":    gaugeType,
			"update_failed":  gaugeType,
			"pdpu_failed":    gaugeType,
			"ue_failed":      gaugeType,
		},
		KeyPeriod: time.Minute,
	}
)

// convertInt64
func (d *Device) convertInt64(m map[string]interface{}, name string) int64 {
	val, ok := m[name]
	if !ok {
		glog.Warn("convertInt64: province: %s device %d Not found key %s\n",
			d.Province, d.Pid, name)
		return 0
	}
	return int64(val.(float64))
}

// convertFloat64
func (d *Device) convertFloat64(m map[string]interface{}, name string) float64 {
	val, ok := m[name]
	if !ok {
		glog.Warn("convertInt64: province: %s device %d Not found key %s\n",
			d.Province, d.Pid, name)
		return 0.0
	}
	return val.(float64)
}

func (d *Device) processPDP(record map[string]interface{}) {
	dm := metrics.GetOrRegisterDataMap("pdp-"+d.Addr, d.r, pdpOpt)

	dm.UpdateInt64("pdpc_total", d.convertInt64(record, "pdpc_total"))
	dm.UpdateInt64("pdpc_failed", d.convertInt64(record, "pdpc_failed"))
	dm.UpdateInt64("pdpc_online", d.convertInt64(record, "pdpc_online"))
	dm.UpdateInt64("wait_list", d.convertInt64(record, "wait_list"))
	dm.UpdateInt64("wait_timeout", d.convertInt64(record, "wait_timeout"))
	dm.UpdateInt64("update_total", d.convertInt64(record, "update_total"))
	dm.UpdateInt64("update_failed", d.convertInt64(record, "update_failed"))
	dm.UpdateInt64("update_waiting", d.convertInt64(record, "update_waiting"))
	dm.UpdateInt64("update_waiting_timeout", d.convertInt64(record, "update_waiting_timeout"))
	dm.UpdateInt64("pdpu_failed", d.convertInt64(record, "pdpu_failed"))
	dm.UpdateInt64("sec_failed", d.convertInt64(record, "sec_failed"))
	dm.UpdateInt64("ue_failed", d.convertInt64(record, "ue_failed"))
}

```

## PeriodCounter

```go
pc: = metrics.GetOrRegisterPeriodCounter("periodCounter", nil)
pc.SetPeriod(metrics.M1)  // 1 minute
pc.SetPeriod(metrics.M5)  // 5 minute
pc.SetPeriod(metrics.M15) // 15 minute
pc.SetPeriod(metrics.H1)  // 1 hour
pc.SetPeriod(metrics.D1)  // 1 day

pc.Inc(60)
pc.Inc(100)

delta, rate := pc.LatestPeriodCountRate(metrics.M1)
delta, rate = pc.LatestPeriodCountRate(metrics.M5)
delta, rate = pc.LatestPeriodCountRate(metrics.M15)
delta, rate = pc.LatestPeriodCountRate(metrics.H1)
....
```
This will report period count and rate

GaugeMap

这个是一种可以计算数值历史和数值之间关系的数据类型，数据关系的计算通过设置自定义函数来完成，
非常灵活。

```go
```

Periodically log every metric in human-readable form to standard error:

```go
go metrics.Log(metrics.DefaultRegistry, 5 * time.Second, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))
```

Periodically log every metric in slightly-more-parseable form to syslog:

```go
w, _ := syslog.Dial("unixgram", "/dev/log", syslog.LOG_INFO, "metrics")
go metrics.Syslog(metrics.DefaultRegistry, 60e9, w)
```

Periodically emit every metric to Graphite using the [Graphite client](https://github.com/cyberdelia/go-metrics-graphite):

```go

import "github.com/cyberdelia/go-metrics-graphite"

addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:2003")
go graphite.Graphite(metrics.DefaultRegistry, 10e9, "metrics", addr)
```

Periodically emit every metric into InfluxDB:

**NOTE:** this has been pulled out of the library due to constant fluctuations
in the InfluxDB API. In fact, all client libraries are on their way out. see
issues [#121](https://github.com/rcrowley/go-metrics/issues/121) and
[#124](https://github.com/rcrowley/go-metrics/issues/124) for progress and details.

```go
import "github.com/guotie/go-metrics-influxdb"

go influxdb.Influxdb(metrics.DefaultRegistry, 10e9, &influxdb.Config{
    Host:     "127.0.0.1:8086",
    Database: "metrics",
    Username: "test",
    Password: "test",
})
```

Periodically upload every metric to Librato using the [Librato client](https://github.com/mihasya/go-metrics-librato):

**Note**: the client included with this repository under the `librato` package
has been deprecated and moved to the repository linked above.

```go
import "github.com/mihasya/go-metrics-librato"

go librato.Librato(metrics.DefaultRegistry,
    10e9,                  // interval
    "example@example.com", // account owner email address
    "token",               // Librato API token
    "hostname",            // source
    []float64{0.95},       // percentiles to send
    time.Millisecond,      // time unit
)
```

Periodically emit every metric to StatHat:

```go
import "github.com/rcrowley/go-metrics/stathat"

go stathat.Stathat(metrics.DefaultRegistry, 10e9, "example@example.com")
```

Maintain all metrics along with expvars at `/debug/metrics`:

This uses the same mechanism as [the official expvar](http://golang.org/pkg/expvar/)
but exposed under `/debug/metrics`, which shows a json representation of all your usual expvars
as well as all your go-metrics.


```go
import "github.com/rcrowley/go-metrics/exp"

exp.Exp(metrics.DefaultRegistry)
```

Installation
------------

```sh
go get github.com/rcrowley/go-metrics
```

StatHat support additionally requires their Go client:

```sh
go get github.com/stathat/go
```

Publishing Metrics
------------------

Clients are available for the following destinations:

* Librato - [https://github.com/mihasya/go-metrics-librato](https://github.com/mihasya/go-metrics-librato)
* Graphite - [https://github.com/cyberdelia/go-metrics-graphite](https://github.com/cyberdelia/go-metrics-graphite)
* InfluxDB - [https://github.com/vrischmann/go-metrics-influxdb](https://github.com/vrischmann/go-metrics-influxdb)
* Ganglia - [https://github.com/appscode/metlia](https://github.com/appscode/metlia)
* Prometheus - [https://github.com/deathowl/go-metrics-prometheus](https://github.com/deathowl/go-metrics-prometheus)
