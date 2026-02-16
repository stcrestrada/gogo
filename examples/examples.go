package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stcrestrada/gogo/v3"
)

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

func ConcurrentGoroutinePoolsWithConcurrentFeed() {
	ctx := context.Background()
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool with new flattened API
	pool := gogo.NewPool(ctx, concurrency, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	})

	// Or listen to a feed of results (concurrent safe)
	feed := pool.Go()
	// read the feed concurrently
	gogo.GoVoid(ctx, func(ctx context.Context) {
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
	ctx := context.Background()
	concurrency := 2
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Simple pool with new flattened API
	pool := gogo.NewPool(ctx, concurrency, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
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
	ctx := context.Background()
	requestConcurrency := 2
	processingConcurrency := 8
	urls := []string{"https://www.reddit.com/", "https://www.apple.com/", "https://www.yahoo.com/", "https://news.ycombinator.com/", "https://httpbin.org/uuid"}

	// Start our request group with new flattened API
	requestGroup := gogo.NewPool(ctx, requestConcurrency, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	})
	requestFeed := requestGroup.Go()

	// Collect request results
	var requestResults []gogo.Optional[*http.Response]
	for result := range requestFeed {
		requestResults = append(requestResults, result)
	}

	// Start our processing group and process the results
	processingGroup := gogo.NewPool(ctx, processingConcurrency, len(requestResults), func(ctx context.Context, i int) (*http.Response, error) {
		// Forward last steps error if there was one
		if requestResults[i].Error != nil {
			return nil, requestResults[i].Error
		}
		doc, err := goquery.NewDocumentFromReader(requestResults[i].Result.Body)
		if err != nil {
			return nil, err
		}
		pageTitle := doc.Find("title").Text()
		fmt.Printf("page %s had title %s \n", requestResults[i].Result.Request.URL.String(), pageTitle)
		return requestResults[i].Result, nil
	})

	// Wait for the pipelines to finish!
	processingGroup.Wait()
}

func CancellationExample() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a pool that will be cancelled after 5 seconds
	pool := gogo.NewPool(ctx, 2, 10, func(ctx context.Context, i int) (string, error) {
		// Simulate long-running work
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
			return fmt.Sprintf("Task %d completed", i), nil
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

func ManualCancellationExample() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := gogo.NewPool(ctx, 2, 10, func(ctx context.Context, i int) (string, error) {
		// Simulate work with different durations
		sleepTime := time.Duration((i + 1)) * time.Second
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(sleepTime):
			return fmt.Sprintf("Task %d completed after %v", i, sleepTime), nil
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

func MapExample() {
	ctx := context.Background()

	// Transform a slice concurrently using Map
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	results, errors := gogo.Map(ctx, 3, numbers, func(ctx context.Context, num int) (int, error) {
		// Simulate some work
		time.Sleep(100 * time.Millisecond)
		return num * num, nil // Square each number
	})

	if len(errors) > 0 {
		fmt.Printf("Encountered %d errors\n", len(errors))
	}

	fmt.Printf("Squared results: %v\n", results)
	fmt.Printf("Processed %d numbers concurrently\n", len(results))
}

func ForEachExample() {
	ctx := context.Background()

	// URLs to process
	urls := []string{
		"https://httpbin.org/delay/1",
		"https://httpbin.org/status/200",
		"https://httpbin.org/uuid",
	}

	// Process each URL concurrently
	errors := gogo.ForEach(ctx, 2, urls, func(ctx context.Context, url string) error {
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("failed to fetch %s: %w", url, err)
		}
		defer resp.Body.Close()

		fmt.Printf("✓ Fetched %s: %d\n", url, resp.StatusCode)
		return nil
	})

	if len(errors) > 0 {
		fmt.Printf("Encountered %d errors:\n", len(errors))
		for _, err := range errors {
			fmt.Printf("  - %v\n", err)
		}
	} else {
		fmt.Println("All URLs processed successfully!")
	}
}

func CollectExample() {
	ctx := context.Background()

	// Create a pool
	pool := gogo.NewPool(ctx, 3, 10, func(ctx context.Context, i int) (int, error) {
		// Simulate some work
		time.Sleep(100 * time.Millisecond)

		// Simulate an error on task 5
		if i == 5 {
			return 0, fmt.Errorf("error on task %d", i)
		}

		return i * 2, nil
	})

	// Use Collect() to get all results at once
	results, errors := pool.Collect()

	fmt.Printf("Completed %d tasks successfully\n", len(results))
	fmt.Printf("Failed %d tasks\n", len(errors))
	fmt.Printf("Results: %v\n", results)
	if len(errors) > 0 {
		fmt.Printf("Errors: %v\n", errors)
	}
}

func main() {
	// Only run quick examples to avoid external HTTP calls in CI/tests
	fmt.Println("\n=== Cancellation Example ===")
	CancellationExample()

	fmt.Println("\n=== Manual Cancellation Example ===")
	ManualCancellationExample()

	fmt.Println("\n=== Map Example ===")
	MapExample()

	fmt.Println("\n=== Collect Example ===")
	CollectExample()

	// Uncomment to run ForEach example (makes external HTTP calls)
	// fmt.Println("\n=== ForEach Example ===")
	// ForEachExample()
}
