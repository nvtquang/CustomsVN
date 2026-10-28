// Package ratelimit is an in-memory token bucket per key: per user for
// questions, per address for logins. One process serves CustomsVN, so the
// buckets need no shared store; a restart forgets them, which errs on the
// side of the user.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter allows PerMinute events a minute per key, with bursts of Burst.
// A zero PerMinute allows everything.
type Limiter struct {
	mu        sync.Mutex
	perMinute float64
	burst     float64
	buckets   map[string]*bucket
	Now       func() time.Time
}

type bucket struct {
	tokens float64
	at     time.Time
}

// New returns a limiter.
func New(perMinute, burst int) *Limiter {
	l := &Limiter{buckets: map[string]*bucket{}}
	l.Set(perMinute, burst)
	return l
}

// Set changes the rate; buckets keep what they hold.
func (l *Limiter) Set(perMinute, burst int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if burst < 1 {
		burst = max(1, perMinute)
	}
	l.perMinute, l.burst = float64(perMinute), float64(burst)
}

func (l *Limiter) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Allow takes one token from key's bucket. When the bucket is empty it
// returns false and how long until a token is back.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.perMinute <= 0 {
		return true, 0
	}
	now := l.now()
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, at: now}
		l.buckets[key] = b
		if len(l.buckets) > 10_000 {
			l.prune(now)
		}
	}
	rate := l.perMinute / 60 // tokens a second
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.at).Seconds()*rate)
	b.at = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / rate * float64(time.Second))
	return false, wait.Round(time.Second) + time.Second
}

// prune forgets buckets that have refilled, so a scan of many addresses does
// not grow the map without end.
func (l *Limiter) prune(now time.Time) {
	full := time.Duration(l.burst / (l.perMinute / 60) * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.at) > full {
			delete(l.buckets, k)
		}
	}
}
