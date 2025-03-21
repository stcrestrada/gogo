# gogo

[![Go](https://github.com/stcrestrada/gogo/actions/workflows/go.yml/badge.svg)](https://github.com/stcrestrada/gogo/actions/workflows/go.yml)

Simple Golang package for async goroutines with pools (workers/semaphores).

## Features

- Simple async function wrapping via `Go` and `GoVoid` functions
- Typed results via generics
- Concurrent goroutine pools with controlled concurrency limits
- Pipeline-style chaining of goroutine pools
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

pool := gogo.NewPool(ctx, 2, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
    url := urls[i]
    return func(ctx context.Context) (*http.Response, error) {
        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            return nil, err
        }
        return http.DefaultClient.Do(req)
    }
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

### Context Cancellation

```go
// Create a context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

pool := gogo.NewPool(ctx, 2, 10, func(i int) func(ctx context.Context) (string, error) {
    return func(ctx context.Context) (string, error) {
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        case <-time.After(2 * time.Second):
            return fmt.Sprintf("Task %d completed", i), nil
        }
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

pool := gogo.NewPool(ctx, 2, 10, func(i int) func(ctx context.Context) (string, error) {
    return func(ctx context.Context) (string, error) {
        // Check for cancellation
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        default:
            // Continue with work
        }
        
        // Do work
        return fmt.Sprintf("Task %d", i), nil
    }
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

// Start processing group and pipe in request results
processingGroup := gogo.NewPool(ctx, processingConcurrency, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
    requestResult := <-requestFeed
    return func(ctx context.Context) (*http.Response, error) {
        if requestResult.Error != nil {
            return nil, requestResult.Error
        }
        // Process the response
        return requestResult.Result, nil
    }
})

// Wait for the pipeline to finish
processingGroup.Wait()
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.