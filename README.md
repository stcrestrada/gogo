# gogo

[![Go](https://github.com/stcrestrada/gogo/actions/workflows/go.yml/badge.svg)](https://github.com/stcrestrada/gogo/actions/workflows/go.yml)

Simple Golang package for async goroutines with pools (workers/semaphores).

## Features

- Simple async function wrapping via `Go` and `GoVoid` functions
- Typed results via generics
- Concurrent goroutine pools with controlled concurrency limits
- **New!** Convenience functions `Map` and `ForEach` for common patterns
- **New!** `Collect()` method for easier result gathering
- **Improved!** Cleaner, flattened API (no more nested functions)
- Context support for proper cancellation and timeout handling
- Easy cancellation of in-progress operations

## Installation

```
go get github.com/stcrestrada/gogo
```

## Basic Usage

### Simple Async Function

```go
import (
    "context"
    "github.com/stcrestrada/gogo"
)

// Create a context
ctx := context.Background()

// Launch in another goroutine (non-blocking)
proc := gogo.Go(ctx, func(ctx context.Context) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", "https://example.com", nil)
    if err != nil {
        return nil, err
    }
    return http.DefaultClient.Do(req)
})

// Do other work...

// Later, wait for results (blocking, concurrency safe)
res, err := proc.Result()
```

### Goroutine Pools with Controlled Concurrency

```go
// Create a context
ctx := context.Background()

// Set up a pool with 2 concurrent goroutines for 5 URLs
urls := []string{"https://example1.com", "https://example2.com", "https://example3.com", "https://example4.com", "https://example5.com"}

pool := gogo.NewPool(ctx, 2, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
    if err != nil {
        return nil, err
    }
    return http.DefaultClient.Do(req)
})

// Get a channel feed of results
feed := pool.Go()

// Process results as they come in
for res := range feed {
    if res.Error != nil {
        fmt.Printf("Error: %v\n", res.Error)
        continue
    }
    fmt.Printf("Got response from %s: %d\n", res.Result.Request.URL, res.Result.StatusCode)
}
```

### Easy Result Collection with Collect()

```go
ctx := context.Background()

pool := gogo.NewPool(ctx, 5, 10, func(ctx context.Context, i int) (int, error) {
    if i == 5 {
        return 0, fmt.Errorf("error on task %d", i)
    }
    return i * 2, nil
})

// Collect() waits for all results and returns separate slices
results, errors := pool.Collect()

fmt.Printf("Got %d results: %v\n", len(results), results)
fmt.Printf("Got %d errors: %v\n", len(errors), errors)
```

### Map - Transform a Slice Concurrently

```go
ctx := context.Background()

// Transform a slice of items concurrently
items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

results, errors := gogo.Map(ctx, 3, items, func(ctx context.Context, item int) (int, error) {
    // Simulate some work
    time.Sleep(100 * time.Millisecond)
    return item * 2, nil
})

fmt.Printf("Results: %v\n", results) // [2, 4, 6, 8, 10, 12, 14, 16, 18, 20]
```

### ForEach - Process Items Concurrently

```go
ctx := context.Background()

urls := []string{"https://example1.com", "https://example2.com", "https://example3.com"}

// Process each URL concurrently (fire-and-forget style)
errors := gogo.ForEach(ctx, 2, urls, func(ctx context.Context, url string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    fmt.Printf("Fetched %s: %d\n", url, resp.StatusCode)
    return nil
})

if len(errors) > 0 {
    fmt.Printf("Encountered %d errors\n", len(errors))
}
```

### Context Cancellation

```go
// Create a context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

pool := gogo.NewPool(ctx, 2, 10, func(ctx context.Context, i int) (string, error) {
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    case <-time.After(2 * time.Second):
        return fmt.Sprintf("Task %d completed", i), nil
    }
})

feed := pool.Go()

// Read results as they come in
for res := range feed {
    if res.Error != nil {
        fmt.Printf("Error: %v\n", res.Error) // Will include context.DeadlineExceeded errors
    } else {
        fmt.Printf("Result: %s\n", res.Result)
    }
}
```

### Manual Cancellation

```go
ctx := context.Background()

pool := gogo.NewPool(ctx, 2, 10, func(ctx context.Context, i int) (string, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    default:
        // Continue with work
    }

    // Do work
    return fmt.Sprintf("Task %d", i), nil
})

feed := pool.Go()

// Some condition to cancel the pool
if someCondition {
    pool.Cancel() // This will cancel all in-progress and pending tasks
}

// Process remaining results (including cancellation errors)
for res := range feed {
    // Handle results
}
```

## Advanced Usage

### Chained Pools (Pipeline)

```go
ctx := context.Background()
requestConcurrency := 2
processingConcurrency := 8
urls := []string{"https://example1.com", "https://example2.com", "https://example3.com"}

// Start request group
requestGroup := gogo.NewPool(ctx, requestConcurrency, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
    if err != nil {
        return nil, err
    }
    return http.DefaultClient.Do(req)
})
requestFeed := requestGroup.Go()

// Collect request results for processing
var requestResults []gogo.Optional[*http.Response]
for result := range requestFeed {
    requestResults = append(requestResults, result)
}

// Start processing group
processingGroup := gogo.NewPool(ctx, processingConcurrency, len(requestResults), func(ctx context.Context, i int) (*http.Response, error) {
    if requestResults[i].Error != nil {
        return nil, requestResults[i].Error
    }
    // Process the response
    return requestResults[i].Result, nil
})

// Wait for the pipeline to finish
processingGroup.Wait()
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.