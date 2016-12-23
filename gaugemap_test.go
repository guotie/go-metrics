package metrics

import "testing"

func TestGaugeMap(t *testing.T) {
	g := GetOrRegisterGaugeMap("pdp", nil)
	g.SetFuncKey("i+j", func(g GaugeMap) int64 {
		sg := g.(*StandardGaugeMap)
		return sg.value["i"].(int64) + sg.value["j"].(int64)
	})
	g.SetFuncKey("i/j", func(g GaugeMap) float64 {
		sg := g.(*StandardGaugeMap)
		v := float64(sg.value["j"].(int64))
		if v == 0.0 {
			return 0.0
		}
		return float64(sg.value["i"].(int64)) / v
	})

	g.SetFuncKey("delta", func(g GaugeMap) int64 {
		sg := g.(*StandardGaugeMap)
		pi, ok := sg.valuePrev["i"]
		if !ok {
			return sg.value["i"].(int64)
		}
		return sg.value["i"].(int64) - pi.(int64)
	})

	g.SetFuncKey("deltaRate", func(g GaugeMap) float64 {
		sg := g.(*StandardGaugeMap)
		// prev value
		pi := sg.valuePrev["i"]
		pj := sg.valuePrev["j"]
		pt := pi.(int64) + pj.(int64)

		ci := sg.value["i"]
		cj := sg.value["j"]
		ct := ci.(int64) + cj.(int64)

		if ct == 0 {
			return 0.0
		}

		if ct < pt {
			return 0.0
		}
		//t.Logf("deltaRate: ci=%v cj=%v ct=%v pi=%v pj=%v pt=%v\n",
		//	ci, cj, ct, pi, pj, pt)
		return float64(ct-pt) / float64(ct)
	})

	g.UpdateInt64("i", 100)
	g.UpdateInt64("j", 400)

	sg := g.Snapshot()
	if sg.ValueInt64("i") != 100 {
		t.Fail()
	}
	if sg.ValueInt64("j") != 400 {
		t.Fail()
	}
	if sg.ValueFloat64("i/j") != 0.25 {
		t.Fail()
	}
	if sg.ValueInt64("i+j") != 500 {
		t.Fail()
	}
	if sg.ValueInt64("delta") != 100 {
		t.Fatalf("delta wrong: %v, expect 100", sg.ValueInt64("delta"))
	}
	if sg.ValueFloat64("deltaRate") != 1.0 {
		t.Fatalf("deltaRate wrong: %v, expect 0.0", sg.ValueFloat64("deltaRate"))
	}

	//
	g.UpdateInt64("i", 300)
	g.UpdateInt64("j", 500)

	sg = g.Snapshot()
	if sg.ValueInt64("i") != 300 {
		t.Fail()
	}
	if sg.ValueInt64("j") != 500 {
		t.Fail()
	}
	if sg.ValueFloat64("i/j") != 0.6 {
		t.Fail()
	}
	if sg.ValueInt64("i+j") != 800 {
		t.Fail()
	}
	if sg.ValueInt64("delta") != 200 {
		t.Fatalf("delta wrong")
	}
	if sg.ValueFloat64("deltaRate") != float64(300)/float64(800) {
		t.Fatalf("deltaRate expect %v, but %v", float64(300)/float64(800),
			sg.ValueFloat64("deltaRate"))
	}

	//
	g.UpdateInt64("i", 50)
	g.UpdateInt64("j", 150)
	sg = g.Snapshot()

	if sg.ValueInt64("i") != 50 {
		t.Fail()
	}
	if sg.ValueInt64("j") != 150 {
		t.Fail()
	}
	if sg.ValueFloat64("i/j") != float64(50)/float64(150) {
		t.Fail()
	}
	if sg.ValueInt64("i+j") != 200 {
		t.Fail()
	}
	if sg.ValueInt64("delta") != 50-300 {
		t.Fatalf("delta wrong")
	}
	if sg.ValueFloat64("deltaRate") != 0.0 {
		t.Fatalf("deltaRate")
	}

}
