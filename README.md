# gogo

[![Go](https://github.com/stcrestrada/gogo/actions/workflows/go.yml/badge.svg)](https://github.com/stcrestrada/gogo/actions/workflows/go.yml)

Simple Golang package for async goroutines with pools (workers/semaphores).

## Features

- Simple async function wrapping via `Go` and `GoVoid`
- Typed results via generics
- Concurrent goroutine pools with controlled concurrency limits
- `Map` and `ForEach` for common concurrent-transform patterns
- `Collect()` returns results in **original submission order**
- `StreamPool` for dynamic work submission (task count not known upfront)
- Fail-fast mode via `WithFailFast()` option
- Context support for cancellation and timeouts
- Zero dependencies (only `goconvey` for tests)

## Installation

```
go get github.com/stcrestrada/gogo/v3
```

## Basic Usage

### Simple Async Function

```go
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

### GoVoid - Fire-and-Forget Async

```go
ctx := context.Background()

// No type parameter needed - returns *Proc[struct{}]
proc := gogo.GoVoid(ctx, func(ctx context.Context) {
    // do some work with no return value
    http.Get("https://example.com")
})

proc.Wait() // block until done
```

### Goroutine Pools with Controlled Concurrency

```go
ctx := context.Background()
urls := []string{"https://example1.com", "https://example2.com", "https://example3.com"}

pool := gogo.NewPool(ctx, 2, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
    if err != nil {
        return nil, err
    }
    return http.DefaultClient.Do(req)
})

// Stream results as they complete
for res := range pool.Go() {
    if res.Error != nil {
        fmt.Printf("Error: %v\n", res.Error)
        continue
    }
    fmt.Printf("Got response: %d\n", res.Result.StatusCode)
}
```

### Collect - Ordered Results

`Collect()` returns results in the **original submission order**, regardless of completion order.

```go
ctx := context.Background()

pool := gogo.NewPool(ctx, 5, 10, func(ctx context.Context, i int) (int, error) {
    if i == 5 {
        return 0, fmt.Errorf("error on task %d", i)
    }
    return i * 2, nil
})

// Results are ordered by task index, errors collected separately
results, errors := pool.Collect()
// results: [0, 2, 4, 6, 8, 10, 12, 14, 16, 18] (without index 5)
```

> **Note:** `Go()`/`Wait()` and `Collect()` are mutually exclusive on a pool. Calling `Collect()` after `Go()` or `Wait()` will panic.

### Map - Transform a Slice Concurrently

Results preserve input order.

```go
ctx := context.Background()

items := []int{1, 2, 3, 4, 5}

results, errors := gogo.Map(ctx, 3, items, func(ctx context.Context, item int) (int, error) {
    return item * 2, nil
})

fmt.Printf("Results: %v\n", results) // [2, 4, 6, 8, 10] - always in input order
```

### ForEach - Process Items Concurrently

```go
ctx := context.Background()

urls := []string{"https://example1.com", "https://example2.com", "https://example3.com"}

errors := gogo.ForEach(ctx, 2, urls, func(ctx context.Context, url string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    fmt.Printf("Fetched %s: %d\n", url, resp.StatusCode)
    return nil
})
```

### Fail-Fast Mode

Cancel remaining tasks on first error using functional options.

```go
ctx := context.Background()

pool := gogo.NewPool(ctx, 2, 10, func(ctx context.Context, i int) (string, error) {
    if i == 0 {
        return "", fmt.Errorf("bad API key")
    }
    // Remaining tasks will see a cancelled context
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    case <-time.After(2 * time.Second):
        return fmt.Sprintf("Task %d", i), nil
    }
}, gogo.WithFailFast())

results, errors := pool.Collect()
```

### StreamPool - Dynamic Work Submission

For when the total number of tasks isn't known upfront (e.g., paginated APIs).

```go
ctx := context.Background()

sp := gogo.NewStreamPool[Page](ctx, 3)

// Fetch first page to discover total
firstPage, _ := fetchPage(ctx, 0)
sp.Submit(func(ctx context.Context) (Page, error) {
    return firstPage, nil
})

// Now enqueue remaining pages based on first page's metadata
for i := 1; i < firstPage.TotalPages; i++ {
    offset := i
    sp.Submit(func(ctx context.Context) (Page, error) {
        return fetchPage(ctx, offset)
    })
}

sp.Close() // signal no more work

results, errors := sp.Collect()
```

`Submit` returns an error if the pool has been closed:

```go
err := sp.Submit(fn)
if errors.Is(err, gogo.ErrPoolClosed) {
    // pool was already closed
}
```

StreamPool also supports `WithFailFast()` and `WithBufferSize()`:

```go
sp := gogo.NewStreamPool[int](ctx, 4, gogo.WithFailFast(), gogo.WithBufferSize(100))
```

### Context Cancellation

```go
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

for res := range pool.Go() {
    if res.Error != nil {
        fmt.Printf("Error: %v\n", res.Error)
    } else {
        fmt.Printf("Result: %s\n", res.Result)
    }
}
```

### Manual Cancellation

```go
pool := gogo.NewPool(ctx, 2, 10, func(ctx context.Context, i int) (string, error) {
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    default:
        return fmt.Sprintf("Task %d", i), nil
    }
})

feed := pool.Go()

pool.Cancel() // cancel all in-progress and pending tasks

for res := range feed {
    // handle results (most will be context.Canceled errors)
}
```

## Advanced Usage

### Chained Pools (Pipeline)

```go
ctx := context.Background()
urls := []string{"https://example1.com", "https://example2.com", "https://example3.com"}

// Stage 1: fetch
requestGroup := gogo.NewPool(ctx, 2, len(urls), func(ctx context.Context, i int) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", urls[i], nil)
    if err != nil {
        return nil, err
    }
    return http.DefaultClient.Do(req)
})

var requestResults []gogo.Optional[*http.Response]
for result := range requestGroup.Go() {
    requestResults = append(requestResults, result)
}

// Stage 2: process
processingGroup := gogo.NewPool(ctx, 8, len(requestResults), func(ctx context.Context, i int) (*http.Response, error) {
    if requestResults[i].Error != nil {
        return nil, requestResults[i].Error
    }
    return requestResults[i].Result, nil
})

processingGroup.Wait()
```

## API Reference

| Type/Function | Description |
|---|---|
| `Go[T](ctx, fn)` | Launch async function, returns `*Proc[T]` |
| `GoVoid(ctx, fn)` | Launch async void function, returns `*Proc[struct{}]` |
| `NewPool[T](ctx, concurrency, size, fn, ...opts)` | Create a fixed-size pool |
| `Map[T, R](ctx, workers, items, fn)` | Transform slice concurrently (ordered results) |
| `ForEach[T](ctx, workers, items, fn)` | Process slice concurrently |
| `NewStreamPool[T](ctx, concurrency, ...opts)` | Create a dynamic submission pool |
| `WithFailFast()` | Pool option: cancel on first error |
| `WithBufferSize(n)` | Pool option: set StreamPool result buffer size |
| `ErrPoolClosed` | Sentinel error from `StreamPool.Submit` after `Close` |

## License

This project is licensed under the MIT License - see the LICENSE file for details.
