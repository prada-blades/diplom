package cache

import "time"

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	DeleteByPrefix(prefix string) error
}
