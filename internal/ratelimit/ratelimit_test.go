package ratelimit

import (
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	l := New(6, 2) // one token every 10 s, two at once
	l.Now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("u1"); !ok {
			t.Fatalf("burst call %d refused", i)
		}
	}
	ok, wait := l.Allow("u1")
	if ok || wait < 9*time.Second || wait > 12*time.Second {
		t.Fatalf("third call: ok %v wait %s", ok, wait)
	}
	if ok, _ := l.Allow("u2"); !ok {
		t.Fatal("another key shares the bucket")
	}
	now = now.Add(10 * time.Second)
	if ok, _ := l.Allow("u1"); !ok {
		t.Fatal("token did not come back")
	}
	l.Set(0, 0)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("u1"); !ok {
			t.Fatal("a zero rate limits")
		}
	}
}
