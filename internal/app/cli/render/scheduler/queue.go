// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"sync"
	"time"
)

// Priority is a scheduling lane label. Lower numeric value = higher
// priority. Three lanes per spec-1.4 D-2 + Q4 ★A.
type Priority uint8

// Priority lanes — exact numeric values are part of the data contract.
const (
	PriorityHigh   Priority = 0
	PriorityNormal Priority = 1
	PriorityIdle   Priority = 2
)

// jobItem carries a Cmd plus the contextual fields needed to populate
// ErrorMsg on recovered panic (spec-1.4 R2 H-6).
type jobItem struct {
	Cmd       Cmd
	CmdID     uint64
	Submitted time.Time
	Priority  Priority
}

// queue is a 3-lane priority FIFO with strict-priority drain
// (high → normal → idle). Slice-based (not heap) per spec-1.4 D-2 + Q4
// ★A — 3 discrete lanes need no heap log(N) machinery.
//
// Capacity: unbounded by design. queue is the backpressure buffer for
// the worker channel (spec-1.4 R2 H-1); callers are expected to
// self-rate-limit (e.g., LLM streaming should batch chunks rather than
// scheduling per-token Cmds).
//
// Starvation by design: PriorityIdle can be starved indefinitely if
// PriorityHigh keeps producing work. spec-1.4a tracker T-A registers
// a fairness-lottery candidate for when this becomes a real problem.
type queue struct {
	mu     sync.Mutex
	high   []jobItem
	normal []jobItem
	idle   []jobItem
}

// newQueue constructs an empty 3-lane priority queue.
func newQueue() *queue { return &queue{} }

// push appends j to its priority lane in FIFO order. Safe for any
// goroutine (mutex-protected); Schedule/ScheduleAt API allows arbitrary
// goroutine push.
func (q *queue) push(j jobItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	switch j.Priority {
	case PriorityHigh:
		q.high = append(q.high, j)
	case PriorityIdle:
		q.idle = append(q.idle, j)
	default:
		// Unknown priorities collapse to Normal; defensive, never expected
		// in practice (Priority enum is closed).
		q.normal = append(q.normal, j)
	}
}

// popIf attempts to submit the highest-priority head while holding the
// queue lock. On submit success it removes exactly that item; on submit
// failure it leaves the item at the same head for a later frame.
//
// This must remain atomic: a split peek()+dropHead() lets a concurrent
// high-priority ScheduleAt insert between the two calls, causing the
// frame loop to delete the newly inserted high-priority item while the
// already-submitted normal item stays queued and later runs twice.
func (q *queue) popIf(submit func(jobItem) bool) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.high) > 0 {
		if !submit(q.high[0]) {
			return false
		}
		q.high[0] = jobItem{}
		q.high = q.high[1:]
		return true
	}
	if len(q.normal) > 0 {
		if !submit(q.normal[0]) {
			return false
		}
		q.normal[0] = jobItem{}
		q.normal = q.normal[1:]
		return true
	}
	if len(q.idle) > 0 {
		if !submit(q.idle[0]) {
			return false
		}
		q.idle[0] = jobItem{}
		q.idle = q.idle[1:]
		return true
	}
	return false
}

// pop is the convenience head-removal method for tests and non-retain use cases.
func (q *queue) pop() (jobItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.high) > 0 {
		j := q.high[0]
		q.high[0] = jobItem{}
		q.high = q.high[1:]
		return j, true
	}
	if len(q.normal) > 0 {
		j := q.normal[0]
		q.normal[0] = jobItem{}
		q.normal = q.normal[1:]
		return j, true
	}
	if len(q.idle) > 0 {
		j := q.idle[0]
		q.idle[0] = jobItem{}
		q.idle = q.idle[1:]
		return j, true
	}
	return jobItem{}, false
}

// len returns the total queued count across lanes. Used by tests +
// optional metric. Goroutine-safe.
func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.high) + len(q.normal) + len(q.idle)
}
