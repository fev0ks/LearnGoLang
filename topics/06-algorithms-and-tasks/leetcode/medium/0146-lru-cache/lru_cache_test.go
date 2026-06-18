package main

import "testing"

func TestLRUCache(t *testing.T) {
	// Сценарий из LeetCode 146 (capacity = 2).
	c := Constructor(2)

	c.Put(1, 1)
	c.Put(2, 2)
	if got := c.Get(1); got != 1 { // 1 становится свежим
		t.Fatalf("Get(1) = %d, want 1", got)
	}
	c.Put(3, 3) // вытесняет ключ 2 (наименее используемый)
	if got := c.Get(2); got != -1 {
		t.Fatalf("Get(2) = %d, want -1 (вытеснен)", got)
	}
	c.Put(4, 4) // вытесняет ключ 1
	if got := c.Get(1); got != -1 {
		t.Fatalf("Get(1) = %d, want -1 (вытеснен)", got)
	}
	if got := c.Get(3); got != 3 {
		t.Fatalf("Get(3) = %d, want 3", got)
	}
	if got := c.Get(4); got != 4 {
		t.Fatalf("Get(4) = %d, want 4", got)
	}
}

func TestLRUCacheUpdateExisting(t *testing.T) {
	c := Constructor(2)
	c.Put(1, 1)
	c.Put(2, 2)
	c.Put(1, 10) // обновление значения существующего ключа + освежение
	c.Put(3, 3)  // должен вытеснить ключ 2, а не 1 (1 был только что использован)

	if got := c.Get(1); got != 10 {
		t.Errorf("Get(1) = %d, want 10", got)
	}
	if got := c.Get(2); got != -1 {
		t.Errorf("Get(2) = %d, want -1 (вытеснен)", got)
	}
	if got := c.Get(3); got != 3 {
		t.Errorf("Get(3) = %d, want 3", got)
	}
}
