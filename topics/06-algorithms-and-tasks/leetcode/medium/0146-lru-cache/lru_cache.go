package main

// LeetCode 146. LRU Cache (Medium)
// https://leetcode.com/problems/lru-cache/
//
// Задача: реализовать кэш с вытеснением по принципу Least Recently Used и
// операциями Get/Put за O(1).
//
// Идея: двусвязный список хранит пары (ключ, значение) в порядке использования
// (фронт — только что обращались, хвост — кандидат на вытеснение), а map даёт
// доступ к нужному элементу списка за O(1). Список берём из container/list.

import (
	"container/list"
	"fmt"
)

// entry — то, что лежит в каждом элементе списка. Ключ нужен, чтобы при
// вытеснении хвоста знать, какую запись удалить из map.
type entry struct {
	key   int
	value int
}

type LRUCache struct {
	capacity int
	order    *list.List            // фронт — свежие, хвост — давно не используемые
	items    map[int]*list.Element // ключ -> элемент списка
}

func Constructor(capacity int) LRUCache {
	return LRUCache{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[int]*list.Element),
	}
}

// Get возвращает значение по ключу и освежает его, либо -1, если ключа нет.
func (c *LRUCache) Get(key int) int {
	el, ok := c.items[key]
	if !ok {
		return -1
	}
	c.order.MoveToFront(el)
	return el.Value.(*entry).value
}

// Put добавляет или обновляет значение, при переполнении вытесняя самый давно
// не используемый ключ.
func (c *LRUCache) Put(key, value int) {
	// Ключ уже есть — обновляем значение и освежаем позицию.
	if el, ok := c.items[key]; ok {
		el.Value.(*entry).value = value
		c.order.MoveToFront(el)
		return
	}

	// Нет места — вытесняем хвост перед вставкой.
	if c.capacity > 0 && len(c.items) >= c.capacity {
		c.evictOldest()
	}

	el := c.order.PushFront(&entry{key: key, value: value})
	c.items[key] = el
}

// evictOldest удаляет самый давно не используемый элемент (хвост списка).
func (c *LRUCache) evictOldest() {
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*entry).key)
}

func main() {
	// Пример из условия LeetCode (capacity = 2).
	cache := Constructor(2)
	cache.Put(1, 1)
	cache.Put(2, 2)
	fmt.Println(cache.Get(1)) // 1
	cache.Put(3, 3)           // вытесняет ключ 2
	fmt.Println(cache.Get(2)) // -1
	cache.Put(4, 4)           // вытесняет ключ 1
	fmt.Println(cache.Get(1)) // -1
	fmt.Println(cache.Get(3)) // 3
	fmt.Println(cache.Get(4)) // 4
}
