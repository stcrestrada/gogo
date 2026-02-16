package gogo

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrPoolClosed is returned by StreamPool.Submit when called after Close.
var ErrPoolClosed = errors.New("gogo: submit on closed StreamPool")

type Optional[T any] struct {
	Result T
	Error  error
}

type Proc[T any] struct {
	fn     func(ctx context.Context) (T, error)
	result *Optional[T]
	done   atomic.Bool
	once   sync.Once
	wg     sync.WaitGroup
	ctx    context.Context
}

func (p *Proc[T]) Done() bool {
	return p.done.Load()
}

// Blocking
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
		p.done.Store(true)
	})
	return p.result.Result, p.result.Error
}

func (p *Proc[T]) Wait() {
	p.Go()
	p.wg.Wait()
}

// GoVoid wraps a function that returns nothing into a Proc.
func GoVoid(ctx context.Context, f func(ctx context.Context)) *Proc[struct{}] {
	wrapper := func(ctx context.Context) (struct{}, error) {
		f(ctx)
		return struct{}{}, nil
	}

	proc := &Proc[struct{}]{
		fn:  wrapper,
		ctx: ctx,
	}
	go proc.Go()
	return proc
}

func (p *Proc[T]) Result() (T, error) {
	return p.Go()
}

func Go[T any](ctx context.Context, fn func(ctx context.Context) (T, error)) *Proc[T] {
	proc := &Proc[T]{
		fn:  fn,
		ctx: ctx,
	}
	go proc.Go()
	return proc
}

// indexedResult is an internal type that pairs a result with its original task index.
type indexedResult[T any] struct {
	index int
	opt   Optional[T]
}

// PoolOption configures optional Pool behavior.
type PoolOption func(*poolConfig)

type poolConfig struct {
	failFast   bool
	bufferSize int // 0 means use default
}

// WithFailFast causes the pool to cancel remaining tasks when any task returns an error.
func WithFailFast() PoolOption {
	return func(c *poolConfig) { c.failFast = true }
}

// WithBufferSize sets the result channel buffer size for StreamPool.
// Defaults to the concurrency level if not specified. Ignored by Pool.
func WithBufferSize(n int) PoolOption {
	return func(c *poolConfig) { c.bufferSize = n }
}

type Pool[T any] struct {
	concurrency int
	size        int
	makeFn      func(ctx context.Context, i int) (T, error)
	feed        chan indexedResult[T] // internal indexed channel
	out         chan Optional[T]      // public unindexed channel (lazy init by Go)
	startOnce   sync.Once
	goOnce      sync.Once
	consumeMode atomic.Int32 // 0=none, 1=Go, 2=Collect
	ctx         context.Context
	cancel      context.CancelFunc
	failFast    bool
}

// start launches the pool workers exactly once. Results are sent to g.feed.
func (g *Pool[T]) start() {
	g.startOnce.Do(func() {
		if g.size == 0 {
			close(g.feed)
			return
		}
		go func() {
			var wg sync.WaitGroup
			wg.Add(g.size)
			guard := make(chan struct{}, g.concurrency)
			for i := 0; i < g.size; i++ {
				guard <- struct{}{}
				taskIndex := i
				go func() {
					defer func() {
						<-guard
						wg.Done()
					}()

					var res T
					var err error

					select {
					case <-g.ctx.Done():
						err = g.ctx.Err()
					default:
						res, err = g.makeFn(g.ctx, taskIndex)
					}

					if err != nil && g.failFast {
						g.cancel()
					}

					g.feed <- indexedResult[T]{
						index: taskIndex,
						opt: Optional[T]{
							Result: res,
							Error:  err,
						},
					}
				}()
			}
			wg.Wait()
			close(g.feed)
		}()
	})
}

func (g *Pool[T]) Cancel() {
	if g.cancel != nil {
		g.cancel()
	}
}

// Go starts the pool and returns a channel of Optional results.
// Results are delivered in completion order through the channel.
// Safe to call multiple times — returns the same channel.
// Panics if Collect() has already been called on this pool.
func (g *Pool[T]) Go() chan Optional[T] {
	if v := g.consumeMode.Load(); v == 2 {
		panic("gogo: Pool.Go() called after Collect()")
	}
	g.consumeMode.Store(1)
	g.start()
	g.goOnce.Do(func() {
		g.out = make(chan Optional[T], g.size)
		go func() {
			for ir := range g.feed {
				g.out <- ir.opt
			}
			close(g.out)
		}()
	})
	return g.out
}

// Wait starts the pool (if not already started) and blocks until all tasks complete.
// Uses Go() internally — calling Collect() after Wait() will panic.
func (g *Pool[T]) Wait() {
	ch := g.Go()
	for range ch {
	}
}

// Collect waits for all tasks to complete and returns slices of results and errors.
// Results are returned in the original submission order (by task index).
// Panics if Go() has already been called or if called more than once.
func (g *Pool[T]) Collect() ([]T, []error) {
	if !g.consumeMode.CompareAndSwap(0, 2) {
		panic("gogo: Pool.Collect() called after Go()/Wait() or called twice")
	}
	g.start()

	indexed := make([]indexedResult[T], 0, g.size)
	for ir := range g.feed {
		indexed = append(indexed, ir)
	}

	// Place results in order by original index
	ordered := make([]Optional[T], g.size)
	for _, ir := range indexed {
		ordered[ir.index] = ir.opt
	}

	results := make([]T, 0, g.size)
	errors := make([]error, 0)
	for _, opt := range ordered {
		if opt.Error != nil {
			errors = append(errors, opt.Error)
		} else {
			results = append(results, opt.Result)
		}
	}

	return results, errors
}

func NewPool[T any](ctx context.Context, concurrency int, size int, fn func(ctx context.Context, i int) (T, error), opts ...PoolOption) *Pool[T] {
	if concurrency > size {
		concurrency = size
	}

	cfg := &poolConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Pool[T]{
		concurrency: concurrency,
		size:        size,
		makeFn:      fn,
		feed:        make(chan indexedResult[T], size),
		ctx:         ctx,
		cancel:      cancel,
		failFast:    cfg.failFast,
	}
}

// Map applies a function to each item in a slice concurrently and returns the results
// Returns a slice of results and a slice of errors (one for each failed item)
func Map[T any, R any](ctx context.Context, workers int, items []T, fn func(ctx context.Context, item T) (R, error)) ([]R, []error) {
	pool := NewPool(ctx, workers, len(items), func(ctx context.Context, i int) (R, error) {
		return fn(ctx, items[i])
	})

	return pool.Collect()
}

// ForEach applies a function to each item in a slice concurrently (fire-and-forget style)
// Returns a slice of errors for any failed operations
func ForEach[T any](ctx context.Context, workers int, items []T, fn func(ctx context.Context, item T) error) []error {
	pool := NewPool(ctx, workers, len(items), func(ctx context.Context, i int) (struct{}, error) {
		err := fn(ctx, items[i])
		return struct{}{}, err
	})

	_, errors := pool.Collect()
	return errors
}

// StreamPool allows dynamic submission of work into a bounded concurrency pool.
// Unlike Pool, the total number of tasks does not need to be known upfront.
type StreamPool[T any] struct {
	ctx         context.Context
	cancel      context.CancelFunc
	concurrency int
	tasks       chan func(context.Context) (T, error)
	results     chan Optional[T]
	done        chan struct{} // closed by Close() to signal no more submissions
	wg          sync.WaitGroup
	closeOnce   sync.Once
	failFast    bool
}

// NewStreamPool creates a StreamPool with the given concurrency limit.
// Call Submit to add work, Close when done submitting, and Results/Collect to read output.
func NewStreamPool[T any](ctx context.Context, concurrency int, opts ...PoolOption) *StreamPool[T] {
	cfg := &poolConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	ctx, cancel := context.WithCancel(ctx)

	bufSize := concurrency
	if cfg.bufferSize > 0 {
		bufSize = cfg.bufferSize
	}

	sp := &StreamPool[T]{
		ctx:         ctx,
		cancel:      cancel,
		concurrency: concurrency,
		tasks:       make(chan func(context.Context) (T, error)),
		results:     make(chan Optional[T], bufSize),
		done:        make(chan struct{}),
		failFast:    cfg.failFast,
	}

	// Start dispatcher that pulls from the tasks channel until done is closed.
	guard := make(chan struct{}, concurrency)
	dispatch := func(fn func(context.Context) (T, error)) {
		guard <- struct{}{}
		sp.wg.Add(1)
		go func() {
			defer func() {
				<-guard
				sp.wg.Done()
			}()

			var res T
			var err error

			select {
			case <-sp.ctx.Done():
				err = sp.ctx.Err()
			default:
				res, err = fn(sp.ctx)
			}

			if err != nil && sp.failFast {
				sp.cancel()
			}

			sp.results <- Optional[T]{
				Result: res,
				Error:  err,
			}
		}()
	}

	go func() {
		for {
			select {
			case fn := <-sp.tasks:
				dispatch(fn)
			case <-sp.done:
				// Drain any tasks that were sent before done was signaled
				for {
					select {
					case fn := <-sp.tasks:
						dispatch(fn)
					default:
						sp.wg.Wait()
						close(sp.results)
						return
					}
				}
			}
		}
	}()

	return sp
}

// Submit adds a task to the pool. Blocks if all workers are busy and the task
// channel is full. Returns ErrPoolClosed if called after Close.
// Safe to call concurrently with Close.
func (sp *StreamPool[T]) Submit(fn func(ctx context.Context) (T, error)) error {
	select {
	case sp.tasks <- fn:
		return nil
	case <-sp.done:
		return ErrPoolClosed
	}
}

// Close signals that no more tasks will be submitted.
// The results channel will be closed once all submitted tasks complete.
func (sp *StreamPool[T]) Close() {
	sp.closeOnce.Do(func() {
		close(sp.done)
	})
}

// Cancel cancels the pool context, causing pending tasks to see a cancelled context.
func (sp *StreamPool[T]) Cancel() {
	sp.cancel()
}

// Results returns the channel of results. Close must eventually be called
// or the channel will never close.
func (sp *StreamPool[T]) Results() <-chan Optional[T] {
	return sp.results
}

// Collect waits for all submitted tasks to complete and returns results and errors.
// Close must be called before Collect, or Collect will block forever.
func (sp *StreamPool[T]) Collect() ([]T, []error) {
	results := make([]T, 0)
	errors := make([]error, 0)
	for opt := range sp.results {
		if opt.Error != nil {
			errors = append(errors, opt.Error)
		} else {
			results = append(results, opt.Result)
		}
	}
	return results, errors
}
