// Package examples demonstrates basic usage of the gogo library.
package examples

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stcrestrada/gogo"
)

// SimpleAsyncGoroutines demonstrates basic async operation with gogo
func SimpleAsyncGoroutines() {
	ctx := context.Background()

	// Launched in another goroutine (not blocking)
	proc := gogo.Go(ctx, func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://news.ycombinator.com/", nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	})

	// wait for results (blocking), concurrent safe
	res, err := proc.Result()
	if err != nil {
		println("err", err)
	}
	println("got status code", res.StatusCode)

	// Or just wait for the results
	gogo.Go(ctx, func(ctx context.Context) (*http.Response, error) {
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

// ConcurrentGoroutinePoolsWithConcurrentFeed demonstrates using a pool with concurrent feed processing
func ConcurrentGoroutinePoolsWithConcurrentFeed() {
	ctx := context.Background()
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool
	pool := gogo.NewPool(ctx, concurrency, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
		url := urls[i]
		return func(ctx context.Context) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultClient.Do(req)
		}
	})

	// Or listen to a feed of results (concurrent safe)
	feed := pool.Go()
	// read the feed concurrently
	gogo.GoVoid[struct{}](ctx, func(ctx context.Context) {
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

// ConcurrentGoroutinePoolsWithRealtimeFeed demonstrates using a pool with realtime feed processing
func ConcurrentGoroutinePoolsWithRealtimeFeed() {
	ctx := context.Background()
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool
	pool := gogo.NewPool(ctx, concurrency, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
		url := urls[i]
		return func(ctx context.Context) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultClient.Do(req)
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

// ChainedPools demonstrates chaining multiple pools together
func ChainedPools() {
	ctx := context.Background()
	requestConcurrency := 2
	processingConcurrency := 8
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Start our request group
	requestGroup := gogo.NewPool(ctx, requestConcurrency, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
		url := urls[i]
		return func(ctx context.Context) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultClient.Do(req)
		}
	})
	requestFeed := requestGroup.Go()

	// Start our processing group and pipe in request results
	processingGroup := gogo.NewPool(ctx, processingConcurrency, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
		requestResult := <-requestFeed
		return func(ctx context.Context) (*http.Response, error) {
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

// CancellationExample demonstrates timeout-based cancellation
func CancellationExample() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a pool that will be cancelled after 5 seconds
	pool := gogo.NewPool(ctx, 2, 10, func(i int) func(ctx context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			// Simulate long-running work
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(2 * time.Second):
				return fmt.Sprintf("Task %d completed", i), nil
			}
		}
	})

	feed := pool.Go()
	completedTasks := 0

	// Read results as they come in
	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Error: %v\n", res.Error)
		} else {
			fmt.Printf("Result: %s\n", res.Result)
			completedTasks++
		}
	}

	fmt.Printf("Completed %d tasks before timeout or cancellation\n", completedTasks)
}

// ManualCancellationExample demonstrates manual cancellation
func ManualCancellationExample() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := gogo.NewPool(ctx, 2, 10, func(i int) func(ctx context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			// Simulate work with different durations
			sleepTime := time.Duration((i + 1)) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(sleepTime):
				return fmt.Sprintf("Task %d completed after %v", i, sleepTime), nil
			}
		}
	})

	feed := pool.Go()

	// Cancel the pool after 3 seconds
	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("Manually cancelling all remaining tasks...")
		pool.Cancel()
	}()

	// Read results as they come in
	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Error: %v\n", res.Error)
		} else {
			fmt.Printf("Result: %s\n", res.Result)
		}
	}

	fmt.Println("All tasks finished or were cancelled")
}

// RunBasicExamples runs all the basic examples
func RunBasicExamples() {
	fmt.Println("\n=== Cancellation Example ===")
	CancellationExample()

	fmt.Println("\n=== Manual Cancellation Example ===")
	ManualCancellationExample()
}
