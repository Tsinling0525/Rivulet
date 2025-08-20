package engine

import (
	"math/rand"
	"time"
)

// RetryPolicy controls retry behavior for node execution
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Jitter     bool
}

func (p RetryPolicy) normalized() RetryPolicy {
	q := p
	if q.BaseDelay <= 0 {
		q.BaseDelay = 200 * time.Millisecond
	}
	if q.MaxDelay <= 0 {
		q.MaxDelay = 5 * time.Second
	}
	if q.MaxDelay < q.BaseDelay {
		q.MaxDelay = q.BaseDelay
	}
	if q.MaxRetries < 0 {
		q.MaxRetries = 0
	}
	return q
}

// backoff returns the backoff duration for a given attempt
func backoff(attempt int, base, max time.Duration, jitter bool) time.Duration {
	d := base << attempt
	if d > max {
		d = max
	}
	if !jitter {
		return d
	}
	// add +/- 50% jitter
	half := d / 2
	delta := time.Duration(rand.Int63n(int64(half))) // #nosec G404 non-crypto
	return half + delta
}
