package main

import (
	"context"
	"log"
	"time"
)

type database interface {
	get(ctx context.Context, key string) (string, error)
	save(ctx context.Context, key, value string) error
}

type entry struct {
	val       string
	expiresAt int64 // unix nano
}

type fakeDB struct{}

func (fakeDB) get(ctx context.Context, key string) (string, error) { return "v:" + key, nil }
func (fakeDB) save(ctx context.Context, key, value string) error   { return nil }

func main() {
	c := newCachedDB(fakeDB{}, time.Second)
	v1, err := c.get(context.Background(), "k")
	log.Println(v1, err)

	sc := newShardCachedDB(fakeDB{}, time.Second)
	v2, err := sc.get(context.Background(), "k")
	log.Println(v2, err)
}
