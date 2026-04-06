package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"
	"unsafe"
)

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	m := map[int]string{0: "kek"}
	time.Sleep(1 * time.Second)
	mlen := 5000000
	mlen++
	mlen--
	mu := sync.Mutex{}
	// Добавим немного данных, чтобы спровоцировать расширение
	go func() {
		for i := 1; i < mlen; i++ {
			mu.Lock()
			m[i+100] = fmt.Sprintf("val%d", i)
			mu.Unlock()
		}
		fmt.Println("done1")
		time.Sleep(1 * time.Second)
	}()
	go func() {
		for i := mlen; i < 2*mlen; i++ {
			mu.Lock()
			m[i+200] = fmt.Sprintf("val%d", i)
			mu.Unlock()
		}
		fmt.Println("done2")
		time.Sleep(1 * time.Second)
	}()
	go func() {
		for i := 2 * mlen; i < 3*mlen; i++ {
			mu.Lock()
			m[i+300] = fmt.Sprintf("val%d", i)
			mu.Unlock()
		}
		fmt.Println("done3")
		time.Sleep(10 * time.Second)
	}()

	delete(m, 42)
	fmt.Println(len(m))

	_ = m
	fmt.Println(len(m))
	select {}
}

//hdr := (*reflect.MapHeader)(unsafe.Pointer(&m))
//h := (*hmap)(unsafe.Pointer(hdr.Data))
//
//fmt.Printf("map: buckets=%p oldbuckets=%p\n", h.buckets, h.oldbuckets)
//if h.oldbuckets != nil {
//	fmt.Println("map is growing, inspecting old bucket 0")
//	oldBuckets := (*[1 << 30]bmap)(h.oldbuckets)
//	b := &oldBuckets[0]
//
//	for i, k := range b.keys() {
//		if k != nil {
//			fmt.Printf("slot[%d] = key: %v\n", i, k)
//		}
//	}
//}

const (
	bucketCnt = 8
)

type hmap struct {
	count     int
	flags     uint8
	B         uint8
	noverflow uint16
	hash0     uint32

	buckets    unsafe.Pointer
	oldbuckets unsafe.Pointer
	nevacuate  uintptr
	// и т.д.
}

type bmap struct {
	tophash [bucketCnt]uint8
	// Следуют ключи, значения и overflow-указатель
}

func (b *bmap) keys() []interface{} {
	ptr := uintptr(unsafe.Pointer(b)) + unsafe.Sizeof(b.tophash)
	keys := make([]interface{}, bucketCnt)
	for i := 0; i < bucketCnt; i++ {
		keyPtr := unsafe.Pointer(ptr + uintptr(i)*unsafe.Sizeof(0))
		keys[i] = *(*interface{})(keyPtr)
	}
	return keys
}
