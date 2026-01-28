package gogo

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSpec(t *testing.T) {
	ctx := context.Background()

	Convey("Given some function makes an http request using Gogo, err and result should return nil", t, func() {
		proc := GoVoid[http.Response](ctx, func(ctx context.Context) {
			req, _ := http.NewRequestWithContext(ctx, "GET", "https://httpbin.org/uuid", nil)
			http.DefaultClient.Do(req)
		})
		proc.Go()
		So(proc.result.Error, ShouldEqual, nil)
		So(proc.result.Result, ShouldResemble, http.Response{})
	})

	Convey("Given some function makes a list of strings and returns a list of ints", t, func() {
		random := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
		proc := Go(ctx, func(ctx context.Context) ([]int, error) {
			var numbers []int
			for _, i := range random {
				conv, err := strconv.Atoi(i)
				if err != nil {
					return nil, err
				}
				numbers = append(numbers, conv)
			}
			return numbers, nil
		})
		res, err := proc.Result()
		So(err, ShouldEqual, nil)
		So(res, ShouldHaveLength, len(random))
	})

	Convey("Given some function makes a list of strings and returns a error", t, func() {
		random := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "one"}
		proc := Go(ctx, func(ctx context.Context) ([]int, error) {
			var numbers []int
			for _, i := range random {
				conv, err := strconv.Atoi(i)
				if err != nil {
					return []int{}, err
				}
				numbers = append(numbers, conv)
			}
			return numbers, nil
		})
		res, err := proc.Result()
		So(err, ShouldNotEqual, nil)
		So(res, ShouldEqual, []int{})
	})

	Convey("Given some function create a pool of 2 concurrent go routines to run", t, func() {
		var numbers []int
		random := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
		group := NewPool(ctx, 2, len(random), func(ctx context.Context, i int) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
				conv, err := strconv.Atoi(random[i])
				if err != nil {
					return 0, err
				}
				return conv, nil
			}
		})
		feed := group.Go()

		for result := range feed {
			if result.Error != nil {
				group.Cancel() // Use new cancellation method
				break
			}

			numbers = append(numbers, result.Result)
		}
		So(numbers, ShouldHaveLength, 10)
	})

	Convey("Given some function create a pool of 25 concurrent workers, with 100 jobs, that should take 2-3s to run", t, func() {
		count := 50
		concurrency := 25
		sleepTime := time.Second * 1
		group := NewPool(ctx, concurrency, count, func(ctx context.Context, i int) (interface{}, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepTime):
				return nil, nil
			}
		})
		start := time.Now()
		group.Wait()
		end := time.Now().Sub(start)
		So(end, ShouldBeBetweenOrEqual, time.Second*2, time.Second*3)
	})

	Convey("Given a Proc, Done() should return true when the result is available", t, func() {
		proc := Go(ctx, func(ctx context.Context) (int, error) {
			return 42, nil
		})
		proc.Wait()
		So(proc.Done(), ShouldBeTrue)
	})

	Convey("Given a Proc, Wait() should block until the result is available", t, func() {
		proc := Go(ctx, func(ctx context.Context) (int, error) {
			time.Sleep(time.Second)
			return 42, nil
		})
		start := time.Now()
		proc.Wait()
		end := time.Now().Sub(start)
		So(end, ShouldBeGreaterThanOrEqualTo, time.Second)
		So(proc.Done(), ShouldBeTrue)
	})

	Convey("Given a Pool, it should handle errors correctly", t, func() {
		group := NewPool(ctx, 2, 5, func(ctx context.Context, i int) (int, error) {
			if i == 3 {
				return 0, errors.New("test error")
			}
			return i, nil
		})
		feed := group.Go()
		var results []int
		var errors []error
		for result := range feed {
			if result.Error != nil {
				errors = append(errors, result.Error)
			} else {
				results = append(results, result.Result)
			}
		}
		So(results, ShouldHaveLength, 4)
		So(errors, ShouldHaveLength, 1)
	})

	Convey("Context cancellation should stop all goroutines", t, func() {
		// Create context with cancelation function
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel() // Ensure cancel is called even if test fails

		group := NewPool(ctx, 2, 10, func(ctx context.Context, i int) (int, error) {
			// First task will complete, others will be slower
			if i == 0 {
				return i, nil
			}

			// Slow tasks will be cancelled
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(3 * time.Second):
				return i, nil
			}
		})

		feed := group.Go()

		// Cancel after first result
		var count int
		for range feed {
			count++
			if count == 1 {
				cancel() // Cancel the context after first result
				break
			}
		}

		// Should still get all results, but most should be cancelled
		var cancelledCount int
		var successCount int

		for result := range feed {
			if result.Error != nil && errors.Is(result.Error, context.Canceled) {
				cancelledCount++
			} else if result.Error == nil {
				successCount++
			}
		}

		So(cancelledCount, ShouldBeGreaterThan, 0)
	})

	Convey("Given a Pool, Collect() should return all results and errors", t, func() {
		group := NewPool(ctx, 2, 5, func(ctx context.Context, i int) (int, error) {
			if i == 3 {
				return 0, errors.New("test error")
			}
			return i * 2, nil
		})
		results, errs := group.Collect()
		So(results, ShouldHaveLength, 4)
		So(errs, ShouldHaveLength, 1)
		// Check that we got the doubled values
		total := 0
		for _, r := range results {
			total += r
		}
		So(total, ShouldEqual, 0+2+4+8) // 0*2 + 1*2 + 2*2 + 4*2 (skipping 3)
	})

	Convey("Map should transform a slice concurrently", t, func() {
		items := []int{1, 2, 3, 4, 5}
		results, errs := Map(ctx, 2, items, func(ctx context.Context, item int) (int, error) {
			return item * 2, nil
		})
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 5)
		// Sum should be (1+2+3+4+5)*2 = 30
		total := 0
		for _, r := range results {
			total += r
		}
		So(total, ShouldEqual, 30)
	})

	Convey("Map should handle errors correctly", t, func() {
		items := []int{1, 2, 3, 4, 5}
		results, errs := Map(ctx, 2, items, func(ctx context.Context, item int) (int, error) {
			if item == 3 {
				return 0, errors.New("error on 3")
			}
			return item * 2, nil
		})
		So(errs, ShouldHaveLength, 1)
		So(results, ShouldHaveLength, 4)
	})

	Convey("ForEach should process all items and return errors", t, func() {
		items := []int{1, 2, 3, 4, 5}
		processed := make([]int, 0, 5)
		var mu sync.Mutex

		errs := ForEach(ctx, 2, items, func(ctx context.Context, item int) error {
			if item == 3 {
				return errors.New("error on 3")
			}
			mu.Lock()
			processed = append(processed, item*2)
			mu.Unlock()
			return nil
		})

		So(errs, ShouldHaveLength, 1)
		So(processed, ShouldHaveLength, 4)
	})

	Convey("Map should work with different types", t, func() {
		items := []string{"1", "2", "3", "4", "5"}
		results, errs := Map(ctx, 3, items, func(ctx context.Context, item string) (int, error) {
			return strconv.Atoi(item)
		})
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 5)
		// Sum should be 1+2+3+4+5 = 15 (order not guaranteed with concurrency)
		total := 0
		for _, r := range results {
			total += r
		}
		So(total, ShouldEqual, 15)
	})
}
