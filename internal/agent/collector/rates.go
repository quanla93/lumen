package collector

import (
	"context"
	"time"
)

// Rates turns the cumulative counters from NetTotals + DiskIOTotals into
// bytes-per-second by diffing successive samples. The agent keeps one
// instance for the life of the process; its zero value is ready to use.
//
// First call: stores the baseline, returns zeros + haveLast=false.
// Subsequent calls: returns (current - previous) / elapsed seconds.
// Counter resets (current < previous, e.g. interface flap or agent
// restart on a busy host with kernel-side counter rollover at 2^32)
// produce zero for that field instead of an absurd negative.
type Rates struct {
	lastNetRx, lastNetTx       uint64
	lastDiskR, lastDiskW       uint64
	lastTime                   time.Time
	haveLast                   bool
}

// RateSample is one tick of net + disk byte rates.
type RateSample struct {
	NetRxBps float64
	NetTxBps float64
	DiskRBps float64
	DiskWBps float64
	First    bool // true on the very first Sample call (rates are 0)
}

// Sample reads current counters and returns the rate since the last call.
// All four sub-collectors are independent — if one fails the others still
// produce a rate.
func (r *Rates) Sample(ctx context.Context, now time.Time) (RateSample, error) {
	netRx, netTx, netErr := NetTotals(ctx)
	diskR, diskW, diskErr := DiskIOTotals(ctx)

	if !r.haveLast {
		r.lastNetRx, r.lastNetTx = netRx, netTx
		r.lastDiskR, r.lastDiskW = diskR, diskW
		r.lastTime = now
		r.haveLast = true
		return RateSample{First: true}, joinErr(netErr, diskErr)
	}

	dt := now.Sub(r.lastTime).Seconds()
	if dt <= 0 {
		// Same-instant sample (clock skew or paused VM); fall back to no
		// new info — keep counters but don't compute rates.
		return RateSample{}, joinErr(netErr, diskErr)
	}

	s := RateSample{
		NetRxBps: rate(netRx, r.lastNetRx, dt),
		NetTxBps: rate(netTx, r.lastNetTx, dt),
		DiskRBps: rate(diskR, r.lastDiskR, dt),
		DiskWBps: rate(diskW, r.lastDiskW, dt),
	}
	r.lastNetRx, r.lastNetTx = netRx, netTx
	r.lastDiskR, r.lastDiskW = diskR, diskW
	r.lastTime = now
	return s, joinErr(netErr, diskErr)
}

func rate(cur, prev uint64, dt float64) float64 {
	if cur < prev {
		// Counter reset / wraparound — emit 0 rather than a huge spike or
		// a negative.
		return 0
	}
	return float64(cur-prev) / dt
}

func joinErr(a, b error) error {
	switch {
	case a != nil && b != nil:
		return a // pick one; caller logs at Warn either way
	case a != nil:
		return a
	default:
		return b
	}
}
