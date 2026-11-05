package sim

import "container/heap"

// Scheduler is a discrete-event scheduler with a virtual clock in milliseconds.
// Events run in (time, seq) order; seq is the insertion counter, so events
// scheduled for the same instant run in the order they were scheduled.
// This total order is what makes every run reproducible.
type Scheduler struct {
	now uint64
	seq uint64
	pq  eventHeap
}

type event struct {
	at  uint64
	seq uint64
	fn  func()
}

type eventHeap []event

func (h eventHeap) Len() int { return len(h) }
func (h eventHeap) Less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	return h[i].seq < h[j].seq
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)   { *h = append(*h, x.(event)) }
func (h *eventHeap) Pop() any {
	old := *h
	e := old[len(old)-1]
	old[len(old)-1] = event{} // drop the closure so it can be garbage collected
	*h = old[:len(old)-1]
	return e
}

func NewScheduler() *Scheduler { return &Scheduler{} }

func (s *Scheduler) Now() uint64 { return s.now }

func (s *Scheduler) Pending() int { return s.pq.Len() }

func (s *Scheduler) At(t uint64, fn func()) {
	if t < s.now {
		t = s.now
	}
	s.seq++
	heap.Push(&s.pq, event{at: t, seq: s.seq, fn: fn})
}

func (s *Scheduler) After(d uint64, fn func()) { s.At(s.now+d, fn) }

func (s *Scheduler) Run(until uint64, stop func() bool) {
	for s.pq.Len() > 0 {
		if s.pq[0].at > until {
			s.now = until
			return
		}
		e := heap.Pop(&s.pq).(event)
		s.now = e.at
		e.fn()
		if stop != nil && stop() {
			return
		}
	}
}
