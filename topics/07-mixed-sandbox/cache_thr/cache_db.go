package main

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type cachedDB struct {
	next database
	ttl  time.Duration
	sf   singleflight.Group

	mu sync.RWMutex
	m  map[string]entry
}

func newCachedDB(next database, ttl time.Duration) *cachedDB {
	c := &cachedDB{next: next, ttl: ttl, m: make(map[string]entry)}
	return c
}

func (c *cachedDB) get(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()

	if ok && time.Now().Unix() < e.expiresAt {
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

func (c *cachedDB) store(key, val string) {
	c.mu.Lock()
	c.m[key] = entry{val: val, expiresAt: time.Now().Add(c.ttl).Unix()}
	c.mu.Unlock()
}

func (c *cachedDB) save(ctx context.Context, key, value string) error {
	if err := c.next.save(ctx, key, value); err != nil {
		return err
	}
	c.sf.Forget(key) // in-flight запрос мог прочитать старое значение
	c.store(key, value)
	return nil
}
