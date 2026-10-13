package controller

import (
	"sync"
	"time"
)

const (
	maxLoginFails   = 5  // per IP+email
	maxIPLoginFails = 30 // per IP
	loginLockFor    = 15 * time.Minute
	loginWindow     = 15 * time.Minute
	maxLimiterKeys  = 50000
	maxEmailLength  = 254
	maxPassLength   = 72
)

type failState struct {
	count int
	first time.Time
	until time.Time
}

type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*failState
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{m: map[string]*failState{}} }

func (l *loginLimiter) blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.m[key]
	if st == nil || st.until.IsZero() {
		return false, 0
	}
	if time.Now().After(st.until) {
		delete(l.m, key)
		return false, 0
	}
	return true, time.Until(st.until)
}

func (l *loginLimiter) fail(key string, limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.m) >= maxLimiterKeys {
		l.pruneLocked(now)
	}
	st := l.m[key]
	if st == nil || now.Sub(st.first) > loginWindow && st.until.IsZero() {
		st = &failState{first: now}
		l.m[key] = st
	}
	st.count++
	if st.count >= limit {
		st.until = now.Add(loginLockFor)
	}
}

// pruneLocked membuang entri yang sudah kedaluwarsa.
func (l *loginLimiter) pruneLocked(now time.Time) {
	for k, st := range l.m {
		if (!st.until.IsZero() && now.After(st.until)) || (st.until.IsZero() && now.Sub(st.first) > loginWindow) {
			delete(l.m, k)
		}
	}
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.m, key)
	l.mu.Unlock()
}
