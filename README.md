# gogo

[![Go](https://github.com/stcrestrada/gogo/actions/workflows/go.yml/badge.svg)](https://github.com/stcrestrada/gogo/actions/workflows/go.yml)

Simple Golang package for async goroutines with pools (workers/semaphores).

## Features

- Simple async function wrapping via `Go` and `GoVoid` functions
- Typed results via generics
- Concurrent goroutine pools with controlled concurrency limits
- Pipeline-style chaining of goroutine pools
- Context support for proper cancellation and timeout handling
- Result transformation and filtering
- Error collection and handling
- Batch processing
- Graceful shutdown support

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

// Cancel all tasks
pool.Cancel() // This will cancel all in-progress and pending tasks

// Process remaining results (including cancellation errors)
for res := range feed {
    // Handle results
}
```

## Advanced Features

### Result Transformation

```go
ctx := context.Background()

// Start with a simple number
proc := gogo.Go(ctx, func(ctx context.Context) (int, error) {
    return 42, nil
})

// Transform using Map
doubled := proc.Map(func(n int) int {
    return n * 2
})

// Transform using Then (with error handling)
validated := doubled.Then(func(n int, err error) (int, error) {
    if err != nil {
        return 0, err
    }
    if n > 100 {
        return 0, errors.New("number too large")
    }
    return n, nil
})

// Transform to a different type
asString := gogo.MapTo(ctx, validated, func(n int) string {
    return "The answer is " + strconv.Itoa(n)
})

// Get final result
result, err := asString.Result()
fmt.Printf("Result: %s\n", result) // Output: Result: The answer is 84
```

### Error Collection

```go
ctx := context.Background()

// Create an error pool that collects errors
pool := gogo.NewErrorPool(ctx, 2, 5, func(i int) func(ctx context.Context) (string, error) {
    return func(ctx context.Context) (string, error) {
        if i%2 == 0 {
            return "", fmt.Errorf("task %d failed", i)
        }
        return fmt.Sprintf("Task %d succeeded", i), nil
    }
})

// Wait for all tasks to complete
pool.Wait()

// Get all errors that occurred
errors := pool.Errors()
fmt.Printf("Collected %d errors\n", len(errors))
for _, err := range errors {
    fmt.Printf("  - %v\n", err)
}
```

### Batch Processing

```go
ctx := context.Background()

// Create a pool with 10 tasks
pool := gogo.NewPool(ctx, 3, 10, func(i int) func(ctx context.Context) (int, error) {
    return func(ctx context.Context) (int, error) {
        return i, nil
    }
})

// Process results in batches of 3
pool.Batch(3, func(batch []gogo.Optional[int]) {
    fmt.Printf("Processing batch of %d items: [", len(batch))
    for i, item := range batch {
        if i > 0 {
            fmt.Print(", ")
        }
        fmt.Print(item.Result)
    }
    fmt.Println("]")
})
```

### Filtering

```go
ctx := context.Background()

// Create a proc with a result
proc := gogo.Go(ctx, func(ctx context.Context) (int, error) {
    return 5, nil
})

// Apply a filter (only allow even numbers)
evenOnly := proc.Filter(func(n int) bool {
    return n%2 == 0 // Only allow even numbers
})

// Get the filtered result
result, err := evenOnly.Result()
if err == gogo.ErrFilterRejected {
    fmt.Println("Number was filtered out (odd)")
} else if err != nil {
    fmt.Printf("Error: %v\n", err)
} else {
    fmt.Printf("Passed filter: %d\n", result)
}
```

### Type-Changing Chained Pools

```go
ctx := context.Background()

// First pool: generate numbers
generatorPool := gogo.NewPool(ctx, 2, 5, func(i int) func(ctx context.Context) (int, error) {
    return func(ctx context.Context) (int, error) {
        return i * 10, nil
    }
})

// Convert the integers to strings in a new pool
stringPool := gogo.Chain(ctx, generatorPool, 3, func(result gogo.Optional[int]) func(ctx context.Context) (string, error) {
    return func(ctx context.Context) (string, error) {
        if result.Error != nil {
            return "", result.Error
        }
        return fmt.Sprintf("Number: %d", result.Result), nil
    }
})

// Process the string results
feed := stringPool.Go()
for res := range feed {
    fmt.Println(res.Result) // "Number: 0", "Number: 10", etc.
}
```

### Graceful Shutdown

```go
ctx := context.Background()

// Create a pool with graceful shutdown (allowing 2 seconds for cleanup)
pool := gogo.NewPool(ctx, 3, 5, func(i int) func(ctx context.Context) (string, error) {
    return func(ctx context.Context) (string, error) {
        select {
        case <-ctx.Done():
            // Graceful shutdown allows tasks to perform cleanup
            fmt.Printf("Task %d cleaning up resources\n", i)
            time.Sleep(500 * time.Millisecond) // Simulate cleanup
            return "", ctx.Err()
        case <-time.After(2 * time.Second):
            return fmt.Sprintf("Task %d completed", i), nil
        }
    }
}).WithGracefulShutdown(2 * time.Second)

// Start the tasks
feed := pool.Go()

// After some time, initiate graceful shutdown
go func() {
    time.Sleep(3 * time.Second)
    pool.Cancel()
}()

// Collect results
for res := range feed {
    // Handle results
}
```

### Enhanced Context Propagation

```go
// Create base context with a value
ctx := context.WithValue(context.Background(), "userID", "user123")

// Create a pool that adds more context values
pool := gogo.NewPool(ctx, 2, 3, func(i int) func(ctx context.Context) (string, error) {
    return func(ctx context.Context) (string, error) {
        // Access context values
        userID, _ := ctx.Value("userID").(string)
        requestID, _ := ctx.Value("requestID").(string)
        return fmt.Sprintf("Task %d for user %s, request %s", i, userID, requestID), nil
    }
}).WithValue("requestID", "req456") // Add request ID to context

// Add timeout
timedPool := pool.WithTimeout(5 * time.Second)

// Execute with enhanced context
feed := timedPool.Go()
for res := range feed {
    fmt.Println(res.Result)
}
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.