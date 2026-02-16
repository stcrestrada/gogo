package gogo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSpec(t *testing.T) {
	ctx := context.Background()

	Convey("Given some function makes an http request using GoVoid, err and result should return nil", t, func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		proc := GoVoid(ctx, func(ctx context.Context) {
			req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
			http.DefaultClient.Do(req)
		})
		res, err := proc.Go()
		So(err, ShouldEqual, nil)
		So(res, ShouldResemble, struct{}{})
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

	Convey("Given a Pool, Collect() should return ordered results and errors", t, func() {
		group := NewPool(ctx, 2, 5, func(ctx context.Context, i int) (int, error) {
			if i == 3 {
				return 0, errors.New("test error")
			}
			return i * 2, nil
		})
		results, errs := group.Collect()
		So(results, ShouldHaveLength, 4)
		So(errs, ShouldHaveLength, 1)
		// Results should be in order: indices 0,1,2,4 (3 errored)
		So(results[0], ShouldEqual, 0)  // 0*2
		So(results[1], ShouldEqual, 2)  // 1*2
		So(results[2], ShouldEqual, 4)  // 2*2
		So(results[3], ShouldEqual, 8)  // 4*2
	})

	Convey("Map should transform a slice concurrently and preserve input order", t, func() {
		items := []int{1, 2, 3, 4, 5}
		results, errs := Map(ctx, 2, items, func(ctx context.Context, item int) (int, error) {
			return item * 2, nil
		})
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 5)
		// Results must be in the same order as input
		So(results[0], ShouldEqual, 2)
		So(results[1], ShouldEqual, 4)
		So(results[2], ShouldEqual, 6)
		So(results[3], ShouldEqual, 8)
		So(results[4], ShouldEqual, 10)
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

	Convey("Map should work with different types and preserve order", t, func() {
		items := []string{"1", "2", "3", "4", "5"}
		results, errs := Map(ctx, 3, items, func(ctx context.Context, item string) (int, error) {
			return strconv.Atoi(item)
		})
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 5)
		So(results[0], ShouldEqual, 1)
		So(results[1], ShouldEqual, 2)
		So(results[2], ShouldEqual, 3)
		So(results[3], ShouldEqual, 4)
		So(results[4], ShouldEqual, 5)
	})

	Convey("Pool with WithFailFast should cancel remaining tasks on first error", t, func() {
		group := NewPool(ctx, 2, 10, func(ctx context.Context, i int) (int, error) {
			if i == 0 {
				return 0, errors.New("immediate failure")
			}
			// Other tasks should see a cancelled context
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(5 * time.Second):
				return i, nil
			}
		}, WithFailFast())

		_, errs := group.Collect()
		So(len(errs), ShouldBeGreaterThan, 0)
	})

	Convey("StreamPool should process dynamically submitted tasks", t, func() {
		sp := NewStreamPool[int](ctx, 2)

		sp.Submit(func(ctx context.Context) (int, error) {
			return 1, nil
		})
		sp.Submit(func(ctx context.Context) (int, error) {
			return 2, nil
		})
		sp.Submit(func(ctx context.Context) (int, error) {
			return 3, nil
		})
		sp.Close()

		results, errs := sp.Collect()
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 3)
		// StreamPool does not guarantee order, check the sum
		total := 0
		for _, r := range results {
			total += r
		}
		So(total, ShouldEqual, 6)
	})

	Convey("StreamPool should handle errors", t, func() {
		sp := NewStreamPool[int](ctx, 2)

		sp.Submit(func(ctx context.Context) (int, error) {
			return 1, nil
		})
		sp.Submit(func(ctx context.Context) (int, error) {
			return 0, errors.New("task failed")
		})
		sp.Submit(func(ctx context.Context) (int, error) {
			return 3, nil
		})
		sp.Close()

		results, errs := sp.Collect()
		So(results, ShouldHaveLength, 2)
		So(errs, ShouldHaveLength, 1)
	})

	Convey("StreamPool with fail-fast should cancel on first error", t, func() {
		sp := NewStreamPool[int](ctx, 1, WithFailFast())

		sp.Submit(func(ctx context.Context) (int, error) {
			return 0, errors.New("fail")
		})
		sp.Submit(func(ctx context.Context) (int, error) {
			// Should see cancelled context due to fail-fast
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
				return 42, nil
			}
		})
		sp.Close()

		_, errs := sp.Collect()
		So(len(errs), ShouldBeGreaterThan, 0)
	})

	Convey("StreamPool Results channel should deliver results as they complete", t, func() {
		sp := NewStreamPool[string](ctx, 2)

		sp.Submit(func(ctx context.Context) (string, error) {
			return "hello", nil
		})
		sp.Submit(func(ctx context.Context) (string, error) {
			return "world", nil
		})
		sp.Close()

		var results []string
		for opt := range sp.Results() {
			So(opt.Error, ShouldBeNil)
			results = append(results, opt.Result)
		}
		So(results, ShouldHaveLength, 2)
	})

	Convey("Pool.Go() should panic if Collect() was already called", t, func() {
		pool := NewPool(ctx, 2, 3, func(ctx context.Context, i int) (int, error) {
			return i, nil
		})
		pool.Collect()
		So(func() { pool.Go() }, ShouldPanic)
	})

	Convey("Pool.Collect() should panic if Go() was already called", t, func() {
		pool := NewPool(ctx, 2, 3, func(ctx context.Context, i int) (int, error) {
			return i, nil
		})
		_ = pool.Go()
		So(func() { pool.Collect() }, ShouldPanic)
	})

	Convey("Pool.Go() can be called multiple times safely", t, func() {
		pool := NewPool(ctx, 2, 3, func(ctx context.Context, i int) (int, error) {
			return i, nil
		})
		ch1 := pool.Go()
		ch2 := pool.Go()
		for range ch1 {
		}
		So(ch1, ShouldEqual, ch2)
	})

	Convey("StreamPool with WithBufferSize should use custom buffer", t, func() {
		sp := NewStreamPool[int](ctx, 1, WithBufferSize(10))
		for i := 0; i < 10; i++ {
			i := i
			sp.Submit(func(ctx context.Context) (int, error) {
				return i, nil
			})
		}
		sp.Close()
		results, errs := sp.Collect()
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 10)
	})

	Convey("Map with empty slice returns empty results", t, func() {
		results, errs := Map(ctx, 2, []int{}, func(ctx context.Context, item int) (int, error) {
			return item, nil
		})
		So(results, ShouldHaveLength, 0)
		So(errs, ShouldHaveLength, 0)
	})

	Convey("Pool.Collect() should panic with clear message after Wait()", t, func() {
		pool := NewPool(ctx, 2, 3, func(ctx context.Context, i int) (int, error) {
			return i, nil
		})
		pool.Wait()
		So(func() { pool.Collect() }, ShouldPanic)
	})

	Convey("StreamPool.Submit after Close returns ErrPoolClosed", t, func() {
		sp := NewStreamPool[int](ctx, 2)
		sp.Close()
		err := sp.Submit(func(ctx context.Context) (int, error) {
			return 1, nil
		})
		So(err, ShouldEqual, ErrPoolClosed)
	})

	Convey("StreamPool.Submit concurrent with Close returns ErrPoolClosed without panic", t, func() {
		for range 50 {
			sp := NewStreamPool[int](ctx, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				sp.Close()
			}()
			// Submit may race with Close — must not panic, should return nil or ErrPoolClosed
			err := sp.Submit(func(ctx context.Context) (int, error) {
				return 1, nil
			})
			<-done
			if err != nil {
				So(err, ShouldEqual, ErrPoolClosed)
			}
			// Drain results so the StreamPool can shut down cleanly
			for range sp.Results() {
			}
		}
	})

	Convey("Collect should preserve order even with varying task durations", t, func() {
		items := []int{5, 4, 3, 2, 1}
		results, errs := Map(ctx, 5, items, func(ctx context.Context, item int) (int, error) {
			// Tasks that come later in the input finish faster
			time.Sleep(time.Duration(item) * 10 * time.Millisecond)
			return item * 10, nil
		})
		So(errs, ShouldHaveLength, 0)
		So(results, ShouldHaveLength, 5)
		// Even though task 5 (50ms) finishes after task 1 (10ms),
		// results must be in input order
		So(results[0], ShouldEqual, 50)
		So(results[1], ShouldEqual, 40)
		So(results[2], ShouldEqual, 30)
		So(results[3], ShouldEqual, 20)
		So(results[4], ShouldEqual, 10)
	})
}
