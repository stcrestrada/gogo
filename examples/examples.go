package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/PuerkitoBio/goquery"
	"github.com/stcrestrada/gogo"
)

func SimpleAsyncGoroutines() {
	// Launched in another goroutine (not blocking)
	proc := gogo.Go(func() (*http.Response, error) {
		return http.Get("https://news.ycombinator.com/")
	})

	// wait for results (blocking), concurrent safe
	res, err := proc.Result()
	if err != nil {
		println("err", err)
	}
	println("got status code", res.StatusCode)

	// Or just wait for the results
	gogo.Go(func() (struct{}, error) {
		// wait for results (blocking), concurrent safe
		res, err := proc.Result()
		if err != nil {
			println("err", err)
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			println("err", err)
		}
		println("got body", body)
		return struct{}{}, nil
	}).Wait()
}

func ConcurrentGoroutinePoolsWithConcurrentFeed() {
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool
	pool := gogo.NewPool(concurrency, len(urls), func(i int) func() (*http.Response, error) {
		url := urls[i]
		return func() (*http.Response, error) {
			resp, err := http.Get(url)
			return resp, err
		}
	})

	// Or listen to a feed of results (concurrent safe)
	feed := pool.Go()
	// read the feed concurrently
	gogo.GoVoid[struct{}](func() {
		for res := range feed {
			if res.Error == nil {
				doc, err := goquery.NewDocumentFromReader(res.Result.Body)
				if err != nil {
					println("unable to parse", err)
					continue
				}
				pageTitle := doc.Find("title").Text()
				fmt.Printf("page %s had title %s \n\n\n", res.Result.Request.URL.String(), pageTitle)
				println("Got response \n", res.Result.StatusCode)
			}
		}
	}).Wait()
}

func ConcurrentGoroutinePoolsWithRealtimeFeed() {
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool
	pool := gogo.NewPool(concurrency, len(urls), func(i int) func() (*http.Response, error) {
		url := urls[i]
		return func() (*http.Response, error) {
			resp, err := http.Get(url)
			return resp, err
		}
	})

	// Or listen to a feed of results (concurrent safe)
	feed := pool.Go()

	// Read the feed
	for res := range feed {
		if res.Error == nil {
			doc, err := goquery.NewDocumentFromReader(res.Result.Body)
			if err != nil {
				println("unable to parse", err)
				continue
			}
			pageTitle := doc.Find("title").Text()
			fmt.Printf("page %s had title %s \n", res.Result.Request.URL.String(), pageTitle)
			println("Got response \n", res.Result.StatusCode)
		}
	}
}

func ChainedPools() {
	requestConcurrency := 2
	processingConcurrency := 8
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Start our request group
	requestGroup := gogo.NewPool(requestConcurrency, len(urls), func(i int) func() (*http.Response, error) {
		url := urls[i]
		return func() (*http.Response, error) {
			return http.Get(url)
		}
	})
	requestFeed := requestGroup.Go()

	// Start our processing group and pipe in request results
	processingGroup := gogo.NewPool(processingConcurrency, len(urls), func(i int) func() (*http.Response, error) {
		requestResult := <-requestFeed
		return func() (*http.Response, error) {
			// Forward last steps error if there was one
			if requestResult.Error != nil {
				return nil, requestResult.Error
			}
			doc, err := goquery.NewDocumentFromReader(requestResult.Result.Body)
			if err != nil {
				return nil, err
			}
			pageTitle := doc.Find("title").Text()
			fmt.Printf("page %s had title %s \n", requestResult.Result.Request.URL.String(), pageTitle)
			return requestResult.Result, nil
		}
	})

	// Wait for the pipelines to finish!
	processingGroup.Wait()
}

func main() {
	ConcurrentGoroutinePoolsWithConcurrentFeed()
	ConcurrentGoroutinePoolsWithRealtimeFeed()
	SimpleAsyncGoroutines()
	ChainedPools()
}
