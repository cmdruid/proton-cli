package fetch

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestTogetherRunsRequestsAtTheSameTime(t *testing.T) {
	var inFlight, peak atomic.Int32
	request := func(context.Context) error {
		if n := inFlight.Add(1); n > peak.Load() {
			peak.Store(n)
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return nil
	}
	if err := Together(context.Background(), request, request, request); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peak.Load() != 3 {
		t.Errorf("%d requests were in flight at once, want 3", peak.Load())
	}
}

func TestTogetherReportsTheFirstFailureInArgumentOrder(t *testing.T) {
	first, second := errors.New("first"), errors.New("second")
	err := Together(context.Background(),
		func(context.Context) error { return nil },
		func(context.Context) error { time.Sleep(20 * time.Millisecond); return first },
		func(context.Context) error { return second },
	)
	if !errors.Is(err, first) {
		t.Errorf("err = %v, want the earlier argument's failure even though it finished last", err)
	}
}

func TestTogetherWaitsForEveryRequest(t *testing.T) {
	var done atomic.Int32
	slow := func(context.Context) error {
		time.Sleep(30 * time.Millisecond)
		done.Add(1)
		return nil
	}
	_ = Together(context.Background(),
		func(context.Context) error { return errors.New("fails immediately") }, slow, slow)
	if done.Load() != 2 {
		t.Errorf("%d of 2 slow requests finished; an early failure must not abandon the rest", done.Load())
	}
}

func TestTogetherWithNothingToDo(t *testing.T) {
	if err := Together(context.Background()); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestMemoLoadsOncePerKey(t *testing.T) {
	var m Memo[string]
	var loads atomic.Int32
	load := func(v string) func() (string, error) {
		return func() (string, error) {
			loads.Add(1)
			return v, nil
		}
	}
	for range 3 {
		got, err := m.Do("a", load("first"))
		if err != nil || got != "first" {
			t.Fatalf("Do = %q, %v", got, err)
		}
	}
	if got, _ := m.Do("b", load("second")); got != "second" {
		t.Errorf("a second key got %q, want its own answer", got)
	}
	if loads.Load() != 2 {
		t.Errorf("loaded %d times, want one per key", loads.Load())
	}
}

func TestMemoRemembersAFailure(t *testing.T) {
	var m Memo[int]
	var loads atomic.Int32
	boom := errors.New("boom")
	for range 2 {
		if _, err := m.Do("", func() (int, error) { loads.Add(1); return 0, boom }); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
	}
	if loads.Load() != 1 {
		t.Errorf("loaded %d times, want a remembered failure", loads.Load())
	}
}

func TestMemoConcurrentCallersForOneKeyLoadOnce(t *testing.T) {
	var m Memo[int]
	var loads atomic.Int32
	load := func() (int, error) {
		loads.Add(1)
		time.Sleep(20 * time.Millisecond)
		return 7, nil
	}
	requests := make([]func(context.Context) error, 8)
	for i := range requests {
		requests[i] = func(context.Context) error {
			got, err := m.Do("shared", load)
			if got != 7 {
				t.Errorf("Do = %d, want 7", got)
			}
			return err
		}
	}
	if err := Together(context.Background(), requests...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loads.Load() != 1 {
		t.Errorf("loaded %d times, want once", loads.Load())
	}
}
