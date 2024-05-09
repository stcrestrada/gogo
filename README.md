# Gogo: launch and manage Goroutines easier

[![CircleCI](https://circleci.com/gh/circleci/circleci-docs.svg?style=svg)](https://app.circleci.com/pipelines/circleci/RB2YFmKNkMC34gwyzkDz4x/CpXcGxTWYD6PtQVvnnWaxH?branch=master)
[![](https://godoc.org/github.com/strestrada/gogo?status.svg)](http://godoc.org/github.com/stcrestrada/gogo)
[![](https://img.shields.io/github/license/stcrestrada/gogo)](https://github.com/stcrestrada/gogo/blob/master/LICENSE)


Manage goroutines and worker pools with ease. Chain them to create complex processing pipelines.   

**Made with ❤**️ in Washington D.C, the original home of [Gogo](https://en.wikipedia.org/wiki/Go-go).

## Features:

- **Easy to use** - Just gogo.Go()!
- **Simple futures (async/await)** - wait for results without making channels
- **Safe** - safe concurrent calls to all methods
- **Concurrency Pools** - run list of work items with n concurrency
- **Results Feeds** - listen to results from work pools as they become available 
- **Chaining** - pipe work pool results into other work pools to create pipelines
 

## Installation

```
$ go get github.com/stcrestrada/gogo
```

## Quick Start

### Simple Async Goroutines

Quickly launch goroutines, wait on them to finish or wait for their results. 
These procs can be accessed concurrently safely!

```go
package main

import (
	"io"
	"net/http"
                        
        "github.com/stcrestrada/gogo"
)

func main() {
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
    gogo.Go(func() (*http.Response, error) {
        // wait for results (blocking), concurrent safe
        resp, err := proc.Result()
        if err != nil {
            println("err", err)
        }
        body, err := io.ReadAll(resp.Body)
        if err != nil {
            println("err", err)
            return nil, err
        }
        println("got body", body)
        return resp, nil
    }).Wait()

}
```                       

### Concurrent Goroutine Pools

Pools allow you to control how concurrently do perform a set of tasks. 

The example shows you how to fetch a list of webpages and read feed concurrently:

```go
package main

import (
    "fmt"
    "net/http"
	
    "github.com/PuerkitoBio/goquery"
    "github.com/stcrestrada/gogo"
)

func main() {
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
     
     // feed of results (concurrent safe)
     feed := pool.Go()
     
     // read the feed concurrently
     gogo.GoVoid[struct{}](func() {
         for res := range feed {
             if res.Error == nil {
                 doc, err := goquery.NewDocumentFromReader(res.Result.Body)
                 if err != nil{
                     println("unable to parse", err)
                     continue
                 }
                 pageTitle := doc.Find("title").Text()
                 fmt.Printf("page %s had title %s \n", res.Result.Request.URL.String(), pageTitle)
                 println("Got response \n", res.Result.StatusCode)
             }
         }
     }).Wait()
}
```

The example is the same as above but reading feed realtime
```go
package main

import (
	"fmt"
	"net/http"

	"github.com/PuerkitoBio/goquery"
	"github.com/stcrestrada/gogo"
)

func main() {
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

	// listen to a feed of results (concurrent safe)
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

```


### Goroutine Pool Chaining

You can chain pools and send the results of one pool to another to create pipelines. Each pool
has its own concurrent worker count to let your pipeline scale dynamically.


```go
package main

import (
    "fmt"
    "net/http"
    
    "github.com/PuerkitoBio/goquery"
    "github.com/stcrestrada/gogo"
)

func main() {
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
        requestResult := <- requestFeed
        return func() (*http.Response, error) {
            // Forward last steps error if there was one
            if requestResult.Error != nil{
                return nil, requestResult.Error
            }
            doc, err := goquery.NewDocumentFromReader(requestResult.Result.Body)
            if err != nil{
                return nil, err
            }
            pageTitle := doc.Find("title").Text()
            fmt.Printf("page %s had title %s \n", requestResult.Request.URL.String(), pageTitle)
            return requestResult.Result, nil
        }
    })
    
    // Wait for the pipelines to finish!
    processingGroup.Wait()

}
```


### Performance

This lib is designed for processes that have a duration in the order of milliseconds. The goal of this 
project is to maximize DevX and safety for concurrent pipelining. 

There should be no significant performance impact for common workloads.

