package infra

import "sync"

type Job struct{ ExecID string }

type Queue interface {
	Push(Job)
	Pop() (Job, bool)
}

type MemQueue struct {
	mu sync.Mutex
	q  []Job
}

func NewMemQueue() *MemQueue { return &MemQueue{} }

func (mq *MemQueue) Push(j Job) { mq.mu.Lock(); defer mq.mu.Unlock(); mq.q = append(mq.q, j) }

func (mq *MemQueue) Pop() (Job, bool) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if len(mq.q) == 0 {
		return Job{}, false
	}
	j := mq.q[0]
	mq.q = mq.q[1:]
	return j, true
}
