package gogo

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Optional represents a result that may contain an error
type Optional[T any] struct {
	Result T
	Error  error
}

// Proc represents a single asynchronous operation that returns a value of type T
type Proc[T any] struct {
	fn     func(ctx context.Context) (T, error)
	result *Optional[T]
	once   sync.Once
	wg     sync.WaitGroup
	ctx    context.Context
}

// Done returns true if the proc has completed execution
func (p *Proc[T]) Done() bool {
	return p.result != nil
}

// Go executes the proc's function if it hasn't been executed yet
// This method is blocking if called multiple times
func (p *Proc[T]) Go() (T, error) {
	p.once.Do(func() {
		p.wg.Add(1)
		resultsChan := make(chan *Optional[T])
		go func() {
			var res T
			var err error

			// Check if context is done before executing
			select {
			case <-p.ctx.Done():
				err = p.ctx.Err()
			default:
				res, err = p.fn(p.ctx)
			}

			resultsChan <- &Optional[T]{
				Result: res,
				Error:  err,
			}
			p.wg.Done()
		}()
		result := <-resultsChan
		p.result = result
	})
	return p.result.Result, p.result.Error
}

// Wait blocks until the proc's function completes
func (p *Proc[T]) Wait() {
	p.Go()
	p.wg.Wait()
}

// Result returns the result of the proc's function, blocking if necessary
func (p *Proc[T]) Result() (T, error) {
	return p.Go()
}

// Then chains a transformation function to execute after this Proc completes
func (p *Proc[T]) Then(f func(T, error) (T, error)) *Proc[T] {
	return Go(p.ctx, func(ctx context.Context) (T, error) {
		res, err := p.Result()
		return f(res, err)
	})
}

// Map transforms the result of this Proc using the given function
func (p *Proc[T]) Map(f func(T) T) *Proc[T] {
	return Go(p.ctx, func(ctx context.Context) (T, error) {
		res, err := p.Result()
		if err != nil {
			return res, err
		}
		return f(res), nil
	})
}

// MapTo transforms the result of this Proc to a different type U
func MapTo[T any, U any](ctx context.Context, p *Proc[T], f func(T) U) *Proc[U] {
	return Go(ctx, func(ctx context.Context) (U, error) {
		res, err := p.Result()
		var u U
		if err != nil {
			return u, err
		}
		return f(res), nil
	})
}

// MapError transforms the error of this Proc using the given function
func (p *Proc[T]) MapError(f func(error) error) *Proc[T] {
	return Go(p.ctx, func(ctx context.Context) (T, error) {
		res, err := p.Result()
		if err != nil {
			return res, f(err)
		}
		return res, nil
	})
}

// Filter returns a new Proc that succeeds only if the predicate returns true
func (p *Proc[T]) Filter(predicate func(T) bool) *Proc[T] {
	return Go(p.ctx, func(ctx context.Context) (T, error) {
		res, err := p.Result()
		if err != nil {
			return res, err
		}
		if !predicate(res) {
			var zero T
			return zero, ErrFilterRejected
		}
		return res, nil
	})
}

// GoVoid wraps a simple function with no return value
func GoVoid[T any](ctx context.Context, f func(ctx context.Context)) *Proc[T] {
	wrapper := func(ctx context.Context) (T, error) {
		f(ctx)
		var t T
		return t, nil
	}

	proc := &Proc[T]{
		fn:  wrapper,
		ctx: ctx,
	}
	go proc.Go()
	return proc
}

// Go creates a new asynchronous operation
func Go[T any](ctx context.Context, fn func(ctx context.Context) (T, error)) *Proc[T] {
	proc := &Proc[T]{
		fn:  fn,
		ctx: ctx,
	}
	go proc.Go()
	return proc
}

// Pool represents a pool of goroutines with controlled concurrency
type Pool[T any] struct {
	concurrency int
	size        int
	makeFn      func(i int) func(ctx context.Context) (T, error)
	feed        chan Optional[T] // Sized to size
	wg          *sync.WaitGroup  // Sized to 1 always
	closeOnce   sync.Once
	startOnce   sync.Once
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc // For cancelling pool workers
	errors      []error            // Collection of errors if ErrorGroup mode is enabled
	errorsMu    sync.Mutex         // Mutex for errors slice
	collectErrs bool               // Whether to collect errors
}

// close marks the pool as closed
func (g *Pool[T]) close() {
	g.closeOnce.Do(func() {
		g.closed = true
		close(g.feed)
		g.wg.Done()
	})
}

// Cancel cancels all in-progress operations in the pool
func (g *Pool[T]) Cancel() {
	if g.cancel != nil {
		g.cancel()
	}
}

// Go starts the execution of the pool
func (g *Pool[T]) Go() chan Optional[T] {
	go g.startOnce.Do(func() {
		var wg = &sync.WaitGroup{}
		wg.Add(g.size)
		guard := make(chan struct{}, g.concurrency)

		for i := 0; i < g.size; i++ {
			guard <- struct{}{}
			fn := g.makeFn(i)
			go func(idx int) {
				defer func() {
					<-guard
					wg.Done()
				}()

				var res T
				var err error

				// Check if pool context is done
				select {
				case <-g.ctx.Done():
					err = g.ctx.Err()
				default:
					res, err = fn(g.ctx)
				}

				// Collect errors if in ErrorGroup mode
				if g.collectErrs && err != nil {
					g.errorsMu.Lock()
					g.errors = append(g.errors, err)
					g.errorsMu.Unlock()
				}

				g.feed <- Optional[T]{
					Result: res,
					Error:  err,
				}
			}(i)
		}
		wg.Wait()
		g.close() // Make sure we close it
	})
	return g.feed
}

// Wait blocks until all operations in the pool complete
func (g *Pool[T]) Wait() {
	g.Go() // Safe to call again in case they haven't!
	g.wg.Wait()
}

// Errors returns all errors collected from the pool if ErrorGroup mode is enabled
func (g *Pool[T]) Errors() []error {
	if !g.collectErrs {
		return nil
	}
	g.errorsMu.Lock()
	defer g.errorsMu.Unlock()
	return append([]error{}, g.errors...)
}

// Batch processes results from the pool in batches of the specified size
func (g *Pool[T]) Batch(batchSize int, fn func([]Optional[T])) {
	feed := g.Go()
	var batch []Optional[T]

	for res := range feed {
		batch = append(batch, res)

		if len(batch) >= batchSize {
			fn(batch)
			batch = nil
		}
	}

	// Process any remaining items
	if len(batch) > 0 {
		fn(batch)
	}
}

// NewPool creates a new pool with the specified concurrency, size, and function factory
func NewPool[T any](ctx context.Context, concurrency int, size int, fn func(i int) func(ctx context.Context) (T, error)) *Pool[T] {
	if concurrency > size {
		concurrency = size
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Create cancellable context if one was provided
	ctx, cancel := context.WithCancel(ctx)

	return &Pool[T]{
		concurrency: concurrency,
		size:        size,
		makeFn:      fn,
		feed:        make(chan Optional[T], size),
		wg:          wg,
		ctx:         ctx,
		cancel:      cancel,
		errors:      make([]error, 0),
	}
}

// NewErrorPool creates a pool that also collects errors during execution
func NewErrorPool[T any](ctx context.Context, concurrency int, size int, fn func(i int) func(ctx context.Context) (T, error)) *Pool[T] {
	pool := NewPool(ctx, concurrency, size, fn)
	pool.collectErrs = true
	return pool
}

// Chain creates a new pool that processes the results of this pool
func Chain[T any, U any](ctx context.Context, pool *Pool[T], concurrency int, fn func(result Optional[T]) func(ctx context.Context) (U, error)) *Pool[U] {
	feed := pool.Go()
	return NewPool(ctx, concurrency, pool.size, func(i int) func(ctx context.Context) (U, error) {
		result := <-feed
		return fn(result)
	})
}

// WithGracefulShutdown adds a graceful shutdown period to the pool
func (g *Pool[T]) WithGracefulShutdown(timeout time.Duration) *Pool[T] {
	originalCancel := g.cancel
	ctx, cancel := context.WithCancel(g.ctx)

	g.cancel = func() {
		// Start a goroutine that will force cancel after timeout
		go func() {
			time.Sleep(timeout)
			originalCancel()
		}()

		// Soft cancel first
		cancel()
	}

	g.ctx = ctx
	return g
}

// ErrFilterRejected is returned when a filter predicate returns false
var ErrFilterRejected = errors.New("value rejected by filter")

// WithTimeout adds a timeout to the pool's context
func (g *Pool[T]) WithTimeout(timeout time.Duration) *Pool[T] {
	timeoutCtx, cancel := context.WithTimeout(g.ctx, timeout)
	originalCancel := g.cancel

	g.ctx = timeoutCtx
	g.cancel = func() {
		cancel()
		originalCancel()
	}

	return g
}

// WithValue adds a value to the pool's context
func (g *Pool[T]) WithValue(key, val interface{}) *Pool[T] {
	g.ctx = context.WithValue(g.ctx, key, val)
	return g
}

// WithCancel adds a cancellation function to the pool
func (g *Pool[T]) WithCancel() (*Pool[T], context.CancelFunc) {
	ctx, cancel := context.WithCancel(g.ctx)
	originalCancel := g.cancel

	g.ctx = ctx
	g.cancel = func() {
		cancel()
		originalCancel()
	}

	return g, cancel
}
