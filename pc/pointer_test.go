package main

import (
	"math"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu    sync.Mutex
	steps []int // x movement of each output step
	times []time.Time
}

func (r *recorder) out(dx, dy int) {
	r.mu.Lock()
	r.steps = append(r.steps, dx)
	r.times = append(r.times, time.Now())
	r.mu.Unlock()
}

func (r *recorder) stats() (total, biggest int, longestPause time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.steps {
		total += s
		if s > biggest {
			biggest = s
		}
		if i > 0 {
			if d := r.times[i].Sub(r.times[i-1]); d > longestPause {
				longestPause = d
			}
		}
	}
	return
}

// feed sends `count` phone moves of dx pixels: `per` moves at a time, every `every`.
func feed(s *smoother, count, per, dx int, every time.Duration) {
	for sent := 0; sent < count; {
		for i := 0; i < per && sent < count; i++ {
			s.add(dx, 0)
			sent++
		}
		time.Sleep(every)
	}
}

func TestSmoothSteadyInput(t *testing.T) {
	r := &recorder{}
	s := newSmoother(r.out, nil)
	feed(s, 30, 1, 8, 16*time.Millisecond) // a steady swipe at 60 Hz
	time.Sleep(300 * time.Millisecond)
	total, biggest, pause := r.stats()
	want := int(math.Round(30 * 8 * accel(8, 0)))
	t.Logf("steady: %d steps, total %d px (want %d), biggest step %d px, longest pause %v", len(r.steps), total, want, biggest, pause)
	if total < want-1 || total > want {
		t.Fatalf("distance %d, want %d", total, want)
	}
	if biggest > 6 {
		t.Fatalf("steps too big (%d px): cursor would jump", biggest)
	}
}

func TestSmoothBurstyWiFi(t *testing.T) {
	r := &recorder{}
	s := newSmoother(r.out, nil)
	feed(s, 30, 3, 8, 48*time.Millisecond) // same swipe, but Wi-Fi delivers 3 moves at once every 48 ms
	time.Sleep(400 * time.Millisecond)
	total, biggest, pause := r.stats()
	want := int(math.Round(30 * 8 * accel(8, 0)))
	t.Logf("bursty: %d steps, total %d px (want %d), biggest step %d px, longest pause %v", len(r.steps), total, want, biggest, pause)
	if total < want-1 || total > want {
		t.Fatalf("distance %d, want %d", total, want)
	}
	// unsmoothed this would be 30 px jumps (3 x 8 x 1.2) with 48 ms of nothing in between
	if biggest > 12 || pause > 25*time.Millisecond {
		t.Fatalf("still jumpy: biggest step %d px, pause %v", biggest, pause)
	}
}

func TestFlushBeforeClick(t *testing.T) {
	r := &recorder{}
	s := newSmoother(r.out, nil)
	s.add(40, 0)
	s.flush()
	total, _, _ := r.stats()
	if want := int(40 * accel(40, 0)); total != want {
		t.Fatalf("flush moved %d, want %d", total, want)
	}
}
