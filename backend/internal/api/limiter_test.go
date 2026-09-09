package api

import (
	"strconv"
	"testing"
	"time"
)

func TestLoginLimiterCountsInFlightAttemptsAtomically(t *testing.T) {
	clock := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	limiter := NewLoginLimiter(LoginLimiterConfig{
		MaxFailures: 2,
		Window:      time.Minute,
		Now:         func() time.Time { return clock },
	})

	first, ok := limiter.BeginAttempt("198.51.100.10")
	if !ok {
		t.Fatal("first attempt was not reserved")
	}
	second, ok := limiter.BeginAttempt("198.51.100.10")
	if !ok {
		t.Fatal("second attempt was not reserved")
	}
	if _, ok := limiter.BeginAttempt("198.51.100.10"); ok {
		t.Fatal("third concurrent attempt bypassed the failure budget")
	}

	limiter.FinishFailure(first)
	limiter.FinishFailure(second)
	if _, ok := limiter.BeginAttempt("198.51.100.10"); ok {
		t.Fatal("peer was not blocked after two failures")
	}

	clock = clock.Add(time.Minute + time.Nanosecond)
	reservation, ok := limiter.BeginAttempt("198.51.100.10")
	if !ok {
		t.Fatal("expired failures were not pruned")
	}
	limiter.FinishNeutral(reservation)
}

func TestLoginLimiterFinishingSuccessClearsPeerState(t *testing.T) {
	limiter := NewLoginLimiter(LoginLimiterConfig{MaxFailures: 2})
	first, ok := limiter.BeginAttempt("198.51.100.11")
	if !ok {
		t.Fatal("first attempt was not reserved")
	}
	limiter.FinishFailure(first)
	second, ok := limiter.BeginAttempt("198.51.100.11")
	if !ok {
		t.Fatal("second attempt was not reserved")
	}
	limiter.FinishSuccess(second)
	if _, ok := limiter.BeginAttempt("198.51.100.11"); !ok {
		t.Fatal("successful peer state was not cleared")
	}
}

func TestLoginLimiterNeutralCompletionReleasesOnlyInFlightSlot(t *testing.T) {
	limiter := NewLoginLimiter(LoginLimiterConfig{MaxFailures: 1})
	reservation, ok := limiter.BeginAttempt("198.51.100.12")
	if !ok {
		t.Fatal("neutral-path attempt was not reserved")
	}
	limiter.FinishNeutral(reservation)
	if _, ok := limiter.BeginAttempt("198.51.100.12"); !ok {
		t.Fatal("neutral completion did not release the in-flight slot")
	}
}

func TestLoginLimiterDoesNotEvictInFlightEntries(t *testing.T) {
	limiter := NewLoginLimiter(LoginLimiterConfig{MaxFailures: 2, MaxEntries: 1})
	active, ok := limiter.BeginAttempt("198.51.100.13")
	if !ok {
		t.Fatal("active attempt was not reserved")
	}
	if _, ok := limiter.BeginAttempt("198.51.100.14"); ok {
		t.Fatal("limiter evicted an in-flight entry to admit a new peer")
	}
	if len(limiter.entries) != 1 {
		t.Fatalf("limiter entry count = %d, want 1", len(limiter.entries))
	}
	limiter.FinishFailure(active)
	if _, ok := limiter.BeginAttempt("198.51.100.14"); !ok {
		t.Fatal("idle entry was not evicted after its attempt finished")
	}
}

func TestLoginLimiterKeepsEntryMapBounded(t *testing.T) {
	limiter := NewLoginLimiter(LoginLimiterConfig{MaxFailures: 1, MaxEntries: 2})
	for index := 0; index < 100; index++ {
		peer := "198.51.100." + strconv.Itoa(index+1)
		reservation, ok := limiter.BeginAttempt(peer)
		if !ok {
			continue
		}
		limiter.FinishFailure(reservation)
		if len(limiter.entries) > 2 {
			t.Fatalf("limiter entry count = %d, exceeded MaxEntries", len(limiter.entries))
		}
	}
	if len(limiter.entries) > 2 {
		t.Fatalf("final limiter entry count = %d, exceeded MaxEntries", len(limiter.entries))
	}
}
