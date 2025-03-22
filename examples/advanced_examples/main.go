package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/stcrestrada/gogo"
)

func ResultTransformationExample() {
	ctx := context.Background()

	// Start with a simple number
	proc := gogo.Go(ctx, func(ctx context.Context) (int, error) {
		return 42, nil
	})

	// Transform using Map
	doubled := proc.Map(func(n int) int {
		return n * 2
	})

	// Transform using Then (allows error handling)
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
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Result: %s\n", result)
}

func ErrorGroupExample() {
	ctx := context.Background()

	// Create an error pool with 3 workers
	pool := gogo.NewErrorPool(ctx, 2, 5, func(i int) func(ctx context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			// Make some tasks fail
			if i%2 == 0 {
				return "", fmt.Errorf("task %d failed", i)
			}
			return fmt.Sprintf("Task %d succeeded", i), nil
		}
	})

	// Wait for all tasks to complete
	pool.Wait()

	// Get all errors
	errors := pool.Errors()
	fmt.Printf("Collected %d errors:\n", len(errors))
	for _, err := range errors {
		fmt.Printf("  - %v\n", err)
	}
}

func BatchProcessingExample() {
	ctx := context.Background()

	// Create a pool with 10 tasks
	pool := gogo.NewPool(ctx, 3, 10, func(i int) func(ctx context.Context) (int, error) {
		return func(ctx context.Context) (int, error) {
			// Simulate work
			time.Sleep(time.Duration(200) * time.Millisecond)
			return i, nil
		}
	})

	// Process results in batches of 3
	fmt.Println("Processing in batches of 3:")
	pool.Batch(3, func(batch []gogo.Optional[int]) {
		fmt.Printf("Got batch of %d items: [", len(batch))
		for i, item := range batch {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(item.Result)
		}
		fmt.Println("]")
		// Simulate batch processing
		time.Sleep(100 * time.Millisecond)
	})
}

func GracefulShutdownExample() {
	ctx := context.Background()

	// Create a pool with graceful shutdown
	pool := gogo.NewPool(ctx, 3, 5, func(i int) func(ctx context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			// Simulate different work durations
			select {
			case <-ctx.Done():
				// Graceful shutdown allows tasks to perform cleanup
				fmt.Printf("Task %d performing cleanup before exit\n", i)
				return "", ctx.Err()
			case <-time.After(time.Duration(i+1) * time.Second):
				return fmt.Sprintf("Task %d completed", i), nil
			}
		}
	}).WithGracefulShutdown(2 * time.Second) // Allow 2 seconds for graceful shutdown

	feed := pool.Go()

	// Start a separate goroutine to cancel after 3 seconds
	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("Initiating graceful shutdown...")
		pool.Cancel()
	}()

	// Collect results
	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Error: %v\n", res.Error)
		} else {
			fmt.Printf("Result: %s\n", res.Result)
		}
	}

	fmt.Println("All tasks completed or cancelled")
}

func ContextPropagationExample() {
	// Create base context
	baseCtx := context.Background()

	// Add a value
	ctx := context.WithValue(baseCtx, "userID", "user123")

	// Create a pool that uses WithValue to add more context data
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

	// Execute
	feed := timedPool.Go()

	// Collect results
	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Error: %v\n", res.Error)
		} else {
			fmt.Printf("%s\n", res.Result)
		}
	}
}

func ChainedPoolsExample() {
	ctx := context.Background()

	// First pool: generate numbers
	generatorPool := gogo.NewPool(ctx, 2, 5, func(i int) func(ctx context.Context) (int, error) {
		return func(ctx context.Context) (int, error) {
			return i * 10, nil
		}
	})

	// Second pool: square the numbers
	squaringPool := gogo.Chain(ctx, generatorPool, 3, func(result gogo.Optional[int]) func(ctx context.Context) (int, error) {
		return func(ctx context.Context) (int, error) {
			if result.Error != nil {
				return 0, result.Error
			}
			return result.Result * result.Result, nil
		}
	})

	// Third pool: convert to strings
	formattingPool := gogo.Chain(ctx, squaringPool, 5, func(result gogo.Optional[int]) func(ctx context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			if result.Error != nil {
				return "", result.Error
			}
			return fmt.Sprintf("The result is %d", result.Result), nil
		}
	})

	// Execute final pool and collect results
	feed := formattingPool.Go()

	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Error: %v\n", res.Error)
		} else {
			fmt.Printf("%s\n", res.Result)
		}
	}
}

func FilteringExample() {
	ctx := context.Background()

	// Create a pool that generates numbers
	pool := gogo.NewPool(ctx, 3, 10, func(i int) func(ctx context.Context) (int, error) {
		return func(ctx context.Context) (int, error) {
			return i, nil
		}
	})

	feed := pool.Go()

	// Use filtering to process only even numbers
	fmt.Println("Processing only even numbers:")
	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Error: %v\n", res.Error)
			continue
		}

		// Create a Proc from the result and filter it
		evenOnly := gogo.Go(ctx, func(ctx context.Context) (int, error) {
			return res.Result, nil
		}).Filter(func(n int) bool {
			return n%2 == 0 // Only allow even numbers
		})

		// Get the filtered result
		result, err := evenOnly.Result()
		if err == gogo.ErrFilterRejected {
			fmt.Printf("Number %d was filtered out (odd)\n", res.Result)
		} else if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Processed even number: %d\n", result)
		}
	}
}

func ErrorSafetyExample() {
	ctx := context.Background()

	// Start with multiple tasks that may fail
	urls := []string{
		"https://www.google.com",
		"https://this-doesnt-exist-at-all.xyz",
		"https://github.com",
	}

	pool := gogo.NewErrorPool(ctx, 2, len(urls), func(i int) func(ctx context.Context) (*http.Response, error) {
		url := urls[i]
		return func(ctx context.Context) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, err
			}
			client := &http.Client{Timeout: 5 * time.Second}
			return client.Do(req)
		}
	})

	// Process results safely, handling errors
	feed := pool.Go()

	var successCount, failureCount int

	for res := range feed {
		if res.Error != nil {
			fmt.Printf("Request failed: %v\n", res.Error)
			failureCount++
		} else {
			fmt.Printf("Request succeeded for %s (status: %d)\n",
				res.Result.Request.URL.String(), res.Result.StatusCode)
			res.Result.Body.Close() // Important: close the body
			successCount++
		}
	}

	fmt.Printf("\nSummary: %d successful requests, %d failures\n",
		successCount, failureCount)

	// Get all collected errors
	errors := pool.Errors()
	fmt.Printf("Errors collected: %d\n", len(errors))
}

func main() {
	fmt.Println("\n=== Result Transformation Example ===")
	ResultTransformationExample()

	fmt.Println("\n=== Error Group Example ===")
	ErrorGroupExample()

	fmt.Println("\n=== Batch Processing Example ===")
	BatchProcessingExample()

	fmt.Println("\n=== Context Propagation Example ===")
	ContextPropagationExample()

	fmt.Println("\n=== Chained Pools Example ===")
	ChainedPoolsExample()

	fmt.Println("\n=== Filtering Example ===")
	FilteringExample()

	fmt.Println("\n=== Error Safety Example ===")
	ErrorSafetyExample()

	fmt.Println("\n=== Graceful Shutdown Example ===")
	GracefulShutdownExample()
}
