package main

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type shard struct {
	mu sync.RWMutex
	m  map[string]entry
	_  [24]byte // padding до 64 байт — против false sharing
}

const shardCount = 256

type shardCachedDB struct {
	next   database
	ttl    time.Duration
	sf     singleflight.Group
	shards [shardCount]shard
}

func newShardCachedDB(next database, ttl time.Duration) *shardCachedDB {
	c := &shardCachedDB{next: next, ttl: ttl}
	for i := range c.shards {
		c.shards[i].m = make(map[string]entry)
	}
	return c
}

func (c *shardCachedDB) shard(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key)) // на hot path лучше свой inline-fnv без аллокации
	return &c.shards[h.Sum32()%shardCount]
}

func (c *shardCachedDB) get(ctx context.Context, key string) (string, error) {
	s := c.shard(key)
	s.mu.RLock()
	e, ok := s.m[key]
	s.mu.RUnlock()

	if ok && time.Now().UnixNano() < e.expiresAt {
		return e.val, nil
	}

	ch := c.sf.DoChan(key, func() (any, error) {
		// отвязываемся от ctx конкретного вызова:
		// иначе отмена первого запроса уронит всех ожидающих
		fetchCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()

		v, err := c.next.get(fetchCtx, key)
		if err != nil {
			return nil, err
		}
		c.store(key, v)
		return v, nil
	})

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			// stale-if-error: отдаём протухшее, если БД лежит
			if ok {
				return e.val, nil
			}
			return "", res.Err
		}
		return res.Val.(string), nil
	}
}

func (c *shardCachedDB) store(key, val string) {
	s := c.shard(key)
	s.mu.Lock()
	s.m[key] = entry{val: val, expiresAt: time.Now().Add(c.ttl).UnixNano()}
	s.mu.Unlock()
}

func (c *shardCachedDB) save(ctx context.Context, key, value string) error {
	if err := c.next.save(ctx, key, value); err != nil {
		return err
	}
	c.sf.Forget(key) // in-flight запрос мог прочитать старое значение
	c.store(key, value)
	return nil
}
