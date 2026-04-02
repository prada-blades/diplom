package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(addr, password string, db int) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Redis{client: client}, nil
}

func (r *Redis) Get(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	value, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	return value, err
}

func (r *Redis) Set(key string, value []byte, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) DeleteByPrefix(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}
