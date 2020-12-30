package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/PuerkitoBio/goquery"
	"github.com/stcrestrada/gogo"
)

func SimpleAsyncGoroutines() {
	// Launched in another goroutine (not blocking)
	proc := gogo.Go(func() (interface{}, error) {
		return http.Get("https://news.ycombinator.com/")
	})

	// Launch multiple goroutines to read off the results!

	// wait for results (blocking), concurrent safe
	res, err := proc.Result()
	if err != nil {
		println("err", err)
	}
	resHttp := res.(*http.Response)
	println("got status code", resHttp.StatusCode)

	// Or just wait for the results
	gogo.Go(func() (interface{}, error) {
		// wait for results (blocking), concurrent safe
		res, err := proc.Result()
		if err != nil {
			println("err", err)
		}
		resHttp := res.(*http.Response)
		body, err := ioutil.ReadAll(resHttp.Body)
		if err != nil {
			println("err", err)
		}
		println("got body", body)
		return nil, nil
	}).Wait()
}

func ConcurrentGoroutinePoolsWithConcurrentFeed() {
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool
	pool := gogo.NewPool(concurrency, len(urls), func(i int) func() (interface{}, error) {
		url := urls[i]
		return func() (interface{}, error) {
			resp, err := http.Get(url)
			return resp, err
		}
	})

	// Or listen to a feed of results (concurrent safe)
	feed := pool.Go()
	// read the feed concurrently
	gogo.GoVoid(func() {
		for res := range feed {
			if res.Error == nil {
				resHttp := res.Result.(*http.Response)
				doc, err := goquery.NewDocumentFromReader(resHttp.Body)
				if err != nil{
					println("unable to parse", err)
					continue
				}
				pageTitle := doc.Find("title").Text()
				fmt.Printf("page %s had title %s \n\n\n", resHttp.Request.URL.String(), pageTitle)
				println("Got response \n", resHttp.StatusCode)
			}
		}
	}).Wait()
}

func ConcurrentGoroutinePoolsWithRealtimeFeed() {
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool
	pool := gogo.NewPool(concurrency, len(urls), func(i int) func() (interface{}, error) {
		url := urls[i]
		return func() (interface{}, error) {
			resp, err := http.Get(url)
			return resp, err
		}
	})

	// Or listen to a feed of results (concurrent safe)
	feed := pool.Go()

	// Read the feed
	for res := range feed {
		if res.Error == nil {
			resHttp := res.Result.(*http.Response)
			doc, err := goquery.NewDocumentFromReader(resHttp.Body)
			if err != nil{
				println("unable to parse", err)
				continue
			}
			pageTitle := doc.Find("title").Text()
			fmt.Printf("page %s had title %s \n", resHttp.Request.URL.String(), pageTitle)
			println("Got response \n", resHttp.StatusCode)
		}
	}
}

func ChainedPools(){
	requestConcurrency := 2
	processingConcurrency := 8
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Start our request group
	requestGroup := gogo.NewPool(requestConcurrency, len(urls), func(i int) func() (interface{}, error) {
		url := urls[i]
		return func() (interface{}, error) {
			return http.Get(url)
		}
	})
	requestFeed := requestGroup.Go()

	// Start our processing group and pipe in request results
	processingGroup := gogo.NewPool(processingConcurrency, len(urls), func(i int) func() (interface{}, error) {
		requestResult := <- requestFeed
		return func() (interface{}, error) {
			// Forward last steps error if there was one
			if requestResult.Error != nil{
				return nil, requestResult.Error
			}
			result := requestResult.Result.(*http.Response)
			doc, err := goquery.NewDocumentFromReader(result.Body)
			if err != nil{
				return nil, err
			}
			pageTitle := doc.Find("title").Text()
			fmt.Printf("page %s had title %s \n", result.Request.URL.String(), pageTitle)
			return nil, nil
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