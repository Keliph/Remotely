package main

// Smooth pointer movement.
//
// The phone sends a small move about 60 times a second, but Wi-Fi often delivers several at once.
// Applying each one as it arrives makes the cursor jump and stutter. Instead, moves are collected and
// played back in small steps every few milliseconds, spread over roughly the time between phone
// updates, so the cursor glides. The spread adapts to how evenly the Wi-Fi delivers.

import (
	"math"
	"sync"
	"time"
)

const pointerTick = 4 * time.Millisecond

type smoother struct {
	mu     sync.Mutex
	px, py float64   // movement still to play back
	fx, fy float64   // sub-pixel remainder
	last   time.Time // when the previous move arrived
	gap    float64   // average seconds between phone moves
	wake   chan struct{}
	out    func(dx, dy int) // moves the real cursor
	start  func()           // called when a burst of movement starts
}

func newSmoother(out func(dx, dy int), start func()) *smoother {
	s := &smoother{wake: make(chan struct{}, 1), gap: 0.016, out: out, start: start}
	go s.run()
	return s
}

// accel: slow finger = precise, fast flick = covers more screen (up to 2x).
func accel(dx, dy int) float64 {
	return 1 + math.Min(math.Hypot(float64(dx), float64(dy)), 40)/40
}

func (s *smoother) add(dx, dy int) {
	now := time.Now()
	f := accel(dx, dy)
	s.mu.Lock()
	if d := now.Sub(s.last).Seconds(); d >= 0 && d < 0.15 {
		s.gap = s.gap*0.8 + d*0.2
	}
	s.last = now
	s.px += float64(dx) * f
	s.py += float64(dy) * f
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// flush plays back everything still pending right away (before a click, so it lands where the cursor
// was heading).
func (s *smoother) flush() {
	s.mu.Lock()
	ix, iy := s.take(s.px, s.py)
	s.mu.Unlock()
	if ix != 0 || iy != 0 {
		s.out(ix, iy)
	}
}

// take moves (sx, sy) from pending into the sub-pixel remainder and returns the whole pixels. mu held.
func (s *smoother) take(sx, sy float64) (int, int) {
	s.px -= sx
	s.py -= sy
	s.fx += sx
	s.fy += sy
	ix, iy := int(s.fx), int(s.fy)
	s.fx -= float64(ix)
	s.fy -= float64(iy)
	return ix, iy
}

// step returns the pixels to move this tick, and whether there was anything pending.
func (s *smoother) step() (int, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.px == 0 && s.py == 0 {
		return 0, 0, false
	}
	tau := math.Max(0.008, math.Min(0.045, s.gap))
	a := 1 - math.Exp(-pointerTick.Seconds()/tau)
	sx, sy := s.px*a, s.py*a
	if math.Abs(s.px) < 0.6 && math.Abs(s.py) < 0.6 {
		sx, sy = s.px, s.py
	}
	ix, iy := s.take(sx, sy)
	return ix, iy, true
}

func (s *smoother) run() {
	for range s.wake {
		if s.start != nil {
			s.start()
		}
		precise(true)
		t := time.NewTicker(pointerTick)
		for idle := 0; idle < 250; { // about a second without movement: stop ticking
			<-t.C
			ix, iy, busy := s.step()
			if ix != 0 || iy != 0 {
				s.out(ix, iy)
			}
			if busy {
				idle = 0
			} else {
				idle++
			}
		}
		t.Stop()
		precise(false)
	}
}
