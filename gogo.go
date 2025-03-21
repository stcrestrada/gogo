package gogo

import (
	"context"
	"sync"
)

type Optional[T any] struct {
	Result T
	Error  error
}

type Proc[T any] struct {
	fn     func(ctx context.Context) (T, error)
	result *Optional[T]
	once   sync.Once
	wg     sync.WaitGroup
	ctx    context.Context
}

func (p *Proc[T]) Done() bool {
	return p.result != nil
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
	})
	return p.result.Result, p.result.Error
}

func (p *Proc[T]) Wait() {
	p.Go()
	p.wg.Wait()
}

// Wrap a simple function
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
}

func (g *Pool[T]) close() {
	g.closeOnce.Do(func() {
		g.closed = true
		close(g.feed)
		g.wg.Done()
	})
}

func (g *Pool[T]) Cancel() {
	if g.cancel != nil {
		g.cancel()
	}
}

func (g *Pool[T]) Go() chan Optional[T] {
	// Close the ability to use the rest of it
	go g.startOnce.Do(func() {
		var wg = &sync.WaitGroup{}
		wg.Add(g.size)
		guard := make(chan struct{}, g.concurrency)
		// Execute the work here
		for i := 0; i < g.size; i++ {
			guard <- struct{}{}
			fn := g.makeFn(i)
			go func() {
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

				g.feed <- Optional[T]{
					Result: res,
					Error:  err,
				}
			}()
		}
		wg.Wait()
		g.close() // Make sure we close it
	})
	return g.feed
}

func (g *Pool[T]) Wait() {
	g.Go() // Safe to call again in case they haven't!
	g.wg.Wait()
}

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
	}
}
