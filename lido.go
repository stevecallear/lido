package lido

import (
	"errors"
	"sync"
	"time"
)

type (
	// Options represents a set of pool options
	Options struct {
		New     func() (interface{}, error)
		Size    int
		Timeout time.Duration
	}

	// Pool represents a pool
	Pool struct {
		items   chan interface{}
		newFn   func() (interface{}, error)
		maxSize int
		curSize int
		timeout time.Duration
		mu      *sync.Mutex
	}

	// Item represents a pool item
	Item struct {
		value   interface{}
		restore func()
		remove  func()
		closed  bool
		mu      *sync.Mutex
	}

	closer interface {
		Close() error
	}
)

// ErrTimeout indicates that a timeout occured waiting for an available item
var ErrTimeout = errors.New("pool: timeout waiting for available item")

// New returns a new pool
func New(o Options) *Pool {
	if o.New == nil {
		panic("pool: new func must not be nil")
	}

	if o.Size <= 0 {
		o.Size = 1
	}

	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}

	return &Pool{
		items:   make(chan interface{}, o.Size),
		maxSize: o.Size,
		timeout: o.Timeout,
		newFn:   o.New,
		mu:      new(sync.Mutex),
	}
}

// Next returns the next available item in the pool
func (p *Pool) Next() (*Item, error) {
	if len(p.items) < 1 {
		if err := p.addNew(); err != nil {
			return nil, err
		}
	}

	select {
	case v := <-p.items:
		return &Item{
			value: v,
			restore: func() {
				p.items <- v
			},
			remove: func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.curSize--
			},
			mu: new(sync.Mutex),
		}, nil
	case <-time.After(p.timeout):
		return nil, ErrTimeout
	}
}

// Size returns the pool size
func (p *Pool) Size() int {
	return p.maxSize
}

// Timeout returns the pool timeout
func (p *Pool) Timeout() time.Duration {
	return p.timeout
}

// Close closes all items in the pool that implement the Closer interface
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	close(p.items)
	for v := range p.items {
		if c, ok := v.(closer); ok {
			if err := c.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Pool) addNew() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.curSize >= p.maxSize {
		return nil
	}

	v, err := p.newFn()
	if err != nil {
		return err
	}

	p.items <- v
	p.curSize++

	return nil
}

// Value returns the item value
func (i *Item) Value() interface{} {
	return i.value
}

// Restore restores the item value
func (i *Item) Restore() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.closed {
		panic("result: already closed")
	}

	i.restore()
	i.closed = true
}

// Remove removes the item value from the pool
func (i *Item) Remove() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.closed {
		panic("result: already closed")
	}

	i.remove()
	i.closed = true
}
