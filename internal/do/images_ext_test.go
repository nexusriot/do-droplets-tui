package do

import (
	"testing"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/metrics"
)

func TestItoa(t *testing.T) {
	cases := map[int]string{
		0:      "0",
		1:      "1",
		42:     "42",
		-7:     "-7",
		123456: "123456",
	}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q want %q", in, got, want)
		}
	}
}

func TestFlattenCPU_NilEmpty(t *testing.T) {
	if got := flattenCPU(nil); got != nil {
		t.Errorf("nil → %v", got)
	}
	if got := flattenCPU(&godo.MetricsResponse{}); got != nil {
		t.Errorf("empty → %v", got)
	}
}

func TestFlattenCPU_IgnoresIdle(t *testing.T) {
	now := metrics.Time(time.Now().UnixMilli())
	resp := &godo.MetricsResponse{
		Data: godo.MetricsData{
			Result: []metrics.SampleStream{
				{
					Metric: metrics.Metric{"mode": "idle"},
					Values: []metrics.SamplePair{
						{Timestamp: now, Value: 999},
					},
				},
				{
					Metric: metrics.Metric{"mode": "user"},
					Values: []metrics.SamplePair{
						{Timestamp: now, Value: 10},
					},
				},
				{
					Metric: metrics.Metric{"mode": "system"},
					Values: []metrics.SamplePair{
						{Timestamp: now, Value: 5},
					},
				},
			},
		},
	}
	got := flattenCPU(resp)
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1 sample", len(got))
	}
	// 10 + 5 = 15; idle excluded.
	if got[0].Value != 15 {
		t.Errorf("value = %v want 15", got[0].Value)
	}
}

func TestFlattenCPU_SortsByTime(t *testing.T) {
	t1 := metrics.Time(2000)
	t2 := metrics.Time(1000)
	t3 := metrics.Time(3000)
	resp := &godo.MetricsResponse{
		Data: godo.MetricsData{
			Result: []metrics.SampleStream{
				{
					Metric: metrics.Metric{"mode": "user"},
					Values: []metrics.SamplePair{
						{Timestamp: t1, Value: 2},
						{Timestamp: t2, Value: 1},
						{Timestamp: t3, Value: 3},
					},
				},
			},
		},
	}
	got := flattenCPU(resp)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].When.After(got[i].When) {
			t.Errorf("not sorted: [%d]=%v before [%d]=%v",
				i-1, got[i-1].When, i, got[i].When)
		}
	}
}
