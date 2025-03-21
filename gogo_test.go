package gogo

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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
		group := NewPool(ctx, 2, len(random), func(i int) func(ctx context.Context) (int, error) {
			ran := random[i]
			return func(ctx context.Context) (int, error) {
				select {
				case <-ctx.Done():
					return 0, ctx.Err()
				default:
					conv, err := strconv.Atoi(ran)
					if err != nil {
						return 0, err
					}
					return conv, nil
				}
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
		group := NewPool(ctx, concurrency, count, func(i int) func(ctx context.Context) (interface{}, error) {
			return func(ctx context.Context) (interface{}, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(sleepTime):
					return nil, nil
				}
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
		group := NewPool(ctx, 2, 5, func(i int) func(ctx context.Context) (int, error) {
			return func(ctx context.Context) (int, error) {
				if i == 3 {
					return 0, errors.New("test error")
				}
				return i, nil
			}
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
		ctx, cancel := context.WithCancel(context.Background())
		
		group := NewPool(ctx, 2, 10, func(i int) func(ctx context.Context) (int, error) {
			return func(ctx context.Context) (int, error) {
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
}