package cache

import (
	"errors"
	"time"
)

var ErrCacheMiss = errors.New("cache miss")

type Noop struct{}

func NewNoop() Noop {
	return Noop{}
}

func (Noop) Get(string) ([]byte, error) {
	return nil, ErrCacheMiss
}

func (Noop) Set(string, []byte, time.Duration) error {
	return nil
}

func (Noop) DeleteByPrefix(string) error {
	return nil
}
