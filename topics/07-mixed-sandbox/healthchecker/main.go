package main

import (
	"context"
	"log"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

var urls = []string{
	"http://google.com",
	"http://non-existent.domain.tld",
	"http://ya.ru",
	"http://ёёёё",
	"http://yandex.ru",
	"https://www.youtube.com",
}

func main() {
	//checkAsync()
	//checkAsyncWithResult()
	checkWithLimit()
}

func checkSync() {
	for _, url := range urls {
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("%s: %v", url, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("%s: %d", url, resp.StatusCode)
		}
		log.Printf("%s: %d", url, resp.StatusCode)
	}
}

func checkAsync() {
	wg := sync.WaitGroup{}
	wg.Add(len(urls))
	for _, url := range urls {
		go func(url string) {
			defer wg.Done()
			result := checkUrl(url)
			if result.IsHealthy {
				log.Printf("%s: %t is healthy", url, result.IsHealthy)
			}
		}(url)
	}

	wg.Wait()
}

func checkAsyncWithResult() {
	result := make(chan Result, len(urls))

	wg := sync.WaitGroup{}
	wg.Add(len(urls))
	for _, url := range urls {
		go func(url string) {
			defer wg.Done()
			result <- checkUrl(url)
		}(url)
	}

	go func() {
		wg.Wait()
		close(result)
	}()

	for v := range result {
		if v.IsHealthy {
			log.Printf("%s: %t is healthy", v.Url, v.IsHealthy)
		}
	}

}

func checkWithLimit() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	urlsChan := producerWithCtx(ctx, urls)

	checker := NewChecker(&http.Client{})

	result := checker.Check(ctx, urlsChan)

	for res := range result {
		switch res.Status {
		case StatusOk:
			log.Printf("%s: ok", res.Url)
		case StatusNotOk:
			log.Printf("%s: not ok", res.Url)
		case StatusCanceled:
			log.Printf("%s: canceled", res.Url)
		}
	}

}

func producer(urls []string) <-chan string {
	result := make(chan string)
	go func() {
		defer close(result)
		for _, url := range urls {
			result <- url
		}
	}()

	return result
}

func producerWithCtx(ctx context.Context, urls []string) <-chan string {
	result := make(chan string)
	go func() {
		defer close(result)
		for _, url := range urls {
			select {
			case <-ctx.Done():
				return
			case result <- url:
			}
		}
	}()

	return result
}

type Result struct {
	IsHealthy bool
	Url       string
}

func checkUrl(url string) Result {
	val := rand.IntN(100)
	time.Sleep(time.Millisecond * time.Duration(val))
	resp, err := http.Get(url)
	if err != nil {
		return Result{IsHealthy: false}
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{Url: url, IsHealthy: false}
	}
	return Result{Url: url, IsHealthy: true}
}
