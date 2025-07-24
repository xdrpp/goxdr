package stat

import (
	"fmt"
	"time"
)

type LatencyMeter struct {
	ctr Counter
}

type LatencyMeterSpan struct {
	meter *LatencyMeter
	start time.Time
}

func (x *LatencyMeter) Start() LatencyMeterSpan {
	return LatencyMeterSpan{
		meter: x,
		start: time.Now(),
	}
}

func (x *LatencyMeter) StartAt(s time.Time) LatencyMeterSpan {
	return LatencyMeterSpan{
		meter: x,
		start: s,
	}
}

func (x LatencyMeterSpan) Stop() {
	x.meter.Add(time.Since(x.start))
}

func (x *LatencyMeter) Add(d time.Duration) int {
	return x.ctr.Add(float64(d.Nanoseconds()))
}

func (x *LatencyMeter) Average() time.Duration {
	return time.Duration(x.ctr.Average())
}

func (x *LatencyMeter) Min() time.Duration {
	return time.Duration(x.ctr.Min())
}

func (x *LatencyMeter) Max() time.Duration {
	return time.Duration(x.ctr.Max())
}

func (x *LatencyMeter) Stddev() time.Duration {
	return time.Duration(x.ctr.Stddev())
}

func (x *LatencyMeter) String() string {
	return fmt.Sprintf("%s±%s [%s:%s]",
		fmtDur(x.Average()), fmtDur(x.Stddev()), fmtDur(x.Min()), fmtDur(x.Max()))
}

type LatencyStats struct {
	Summary string
	NSec    CounterStats
}

func (x LatencyStats) Summarize() string {
	return fmt.Sprintf("%s±%s [%s:%s]",
		fmtDur(time.Duration(x.NSec.Average)),
		fmtDur(time.Duration(x.NSec.Stddev)),
		fmtDur(time.Duration(x.NSec.Min)),
		fmtDur(time.Duration(x.NSec.Max)),
	)
}

func (x *LatencyMeter) Stats() LatencyStats {
	s := LatencyStats{NSec: x.ctr.Stats()}
	s.Summary = s.Summarize()
	return s
}

func fmtDur(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%.1fh", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int64(d.Minutes()))
	case d >= time.Second:
		return fmt.Sprintf("%ds", int64(d.Seconds()))
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", int64(d.Milliseconds()))
	case d >= time.Microsecond:
		return fmt.Sprintf("%dµs", int64(d.Microseconds()))
	default:
		return fmt.Sprintf("%dns", int64(d.Nanoseconds()))
	}
}
