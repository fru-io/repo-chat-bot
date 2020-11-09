/*
Copyright DDEV Technologies LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
