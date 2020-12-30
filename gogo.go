package gogo

import (
	"sync"
)

type Optional struct {
	Result interface{}
	Error  error
}

type Proc struct {
	fn     func() (interface{}, error)
	result *Optional
	once   sync.Once
	wg     sync.WaitGroup
}

func (p *Proc) Done() bool {
	return p.result != nil
}

// Blocking
func (p *Proc) Go() (interface{}, error) {
	p.once.Do(func() {
		p.wg.Add(1)
		resultsChan := make(chan *Optional)
		go func() {
			res, err := p.fn()
			resultsChan <- &Optional{
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


func (p *Proc) Wait(){
	p.Go()
	p.wg.Wait()
}

// Wrap a simple function
func GoVoid(f func()) *Proc {
	wrapper := func() (interface{}, error) {
		f()
		return nil, nil
	}

	proc := &Proc{
		fn: wrapper,
	}
	go proc.Go()
	return proc
}

func (p *Proc) Result() (interface{}, error) {
	return p.Go()
}

func Go(fn func() (interface{}, error)) *Proc {
	proc := &Proc{
		fn: fn,
	}
	go proc.Go()
	return proc
}

type Pool struct {
	concurrency int
	size        int
	makeFn      func(i int) func() (interface{}, error)
	feed        chan Optional   // Sized to size
	wg          *sync.WaitGroup // Sized to 1 always
	closeOnce   sync.Once
	startOnce   sync.Once
	closed      bool
}

func (g *Pool) close() {
	g.closeOnce.Do(func() {
		g.closed = true
		close(g.feed)
		g.wg.Done()
	})
}

func (g *Pool) Go() chan Optional {
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
				res, err := fn()
				g.feed <- Optional{
					Result: res,
					Error:  err,
				}
				<-guard
				wg.Done()
			}()

		}
		wg.Wait()
		g.close() // Make sure we close it
	})
	return g.feed
}

func (g *Pool) Wait() {
	g.Go() // Safe to call again in case they haven't!
	g.wg.Wait()
}

func NewPool(concurrency int, size int, fn func(i int) func() (interface{}, error)) *Pool {
	if concurrency > size {
		concurrency = size
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &Pool{
		concurrency: concurrency,
		size:        size,
		makeFn:      fn,
		feed:        make(chan Optional, size),
		wg:          wg,
	}
}
