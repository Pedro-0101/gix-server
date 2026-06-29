package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastTick time.Time
}

type Store struct {
	mu    sync.Mutex
	users map[int64]*bucket
}

func New() *Store {
	return &Store{users: make(map[int64]*bucket)}
}

func (s *Store) Allow(userID int64, rate, burst int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.users[userID]
	if !ok {
		b = &bucket{tokens: float64(burst), lastTick: time.Now()}
		s.users[userID] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastTick).Seconds()
	b.tokens += elapsed * float64(rate)
	if b.tokens > float64(burst) {
		b.tokens = float64(burst)
	}
	b.lastTick = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
