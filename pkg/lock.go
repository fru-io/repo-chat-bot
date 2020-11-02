package pkg

import (
	"fmt"
	"sync"
)

type PRLock struct {
	l *sync.Mutex
	m map[string]*lockEntry
}

type lockEntry struct {
	l *sync.Mutex
	c int
}

func NewPRLock() *PRLock {
	return &PRLock{
		l: &sync.Mutex{},
		m: make(map[string]*lockEntry),
	}
}

func (l *PRLock) Lock(repo string, pr int) {
	l.l.Lock()
	key := fmt.Sprintf("%v/%v", repo, pr)
	if e, ok := l.m[key]; ok {
		e.c += 1
	} else {
		l.m[key] = &lockEntry{c: 1, l: &sync.Mutex{}}
	}
	l.l.Unlock()
	l.m[key].l.Lock()
}

func (l *PRLock) Unlock(repo string, pr int) {
	l.l.Lock()
	key := fmt.Sprintf("%v/%v", repo, pr)
	e := l.m[key]
	e.c -= 1
	if e.c < 1 {
		delete(l.m, key)
	}
	l.l.Unlock()
	e.l.Unlock()
}
