package lido_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stevecallear/lido"
)

func ExampleNew() {
	pool := lido.New(lido.Options{
		New: func() (interface{}, error) {
			return "value", nil
		},
	})

	item, err := pool.Next()
	if err != nil {
		panic(err)
	}
	defer item.Restore()

	fmt.Println(item.Value())
	// Output: value
}

func TestNew(t *testing.T) {
	factory := newFactory()

	tests := []struct {
		name    string
		options lido.Options
		size    int
		timeout time.Duration
		panic   bool
	}{
		{
			name:    "should panic if new fn is nil",
			options: lido.Options{},
			panic:   true,
		},
		{
			name: "should use a default size",
			options: lido.Options{
				New:     factory.create,
				Timeout: 100 * time.Millisecond,
			},
			size:    1,
			timeout: 100 * time.Millisecond,
		},
		{
			name: "should use a default timeout",
			options: lido.Options{
				New:  factory.create,
				Size: 100,
			},
			size:    100,
			timeout: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r != nil && !tt.panic {
					t.Errorf("got %v, expected nil", r)
				}
				if r == nil && tt.panic {
					t.Error("got nil, expected panic")
				}
			}()

			p := lido.New(tt.options)

			if p.Size() != tt.size {
				t.Errorf("got %d, expected %d", p.Size(), tt.size)
			}

			if p.Timeout() != tt.timeout {
				t.Errorf("got %v, expected %v", p.Timeout(), tt.timeout)
			}
		})
	}
}

func TestPool_Next(t *testing.T) {
	factory := newFactory()

	tests := []struct {
		name     string
		size     int
		newFn    func() (interface{}, error)
		assertFn func(*testing.T, int, *lido.Item, error)
		times    int
	}{
		{
			name: "should return new func errors",
			size: 1,
			newFn: func() (interface{}, error) {
				return nil, errBasic
			},
			assertFn: func(t *testing.T, _ int, _ *lido.Item, err error) {
				if err != errBasic {
					t.Errorf("got %v, expected %v", err, errBasic)
				}
			},
			times: 1,
		},
		{
			name:  "should reuse pool items",
			size:  10,
			newFn: factory.create,
			assertFn: func(t *testing.T, _ int, r *lido.Item, err error) {
				if err != nil {
					t.Errorf("got %v, expected nil", err)
				}
				defer r.Restore()

				exp := 0
				act := r.Value().(*value).id
				if act != exp {
					t.Errorf("got id=%d, expected %d", act, exp)
				}
			},
			times: 2,
		},
		{
			name:  "should grow the pool to the maximum size",
			size:  2,
			newFn: factory.create,
			assertFn: func(t *testing.T, n int, r *lido.Item, err error) {
				if err != nil {
					t.Errorf("got %v, expected nil", err)
				}
				// don't restore the item

				exp := n
				act := r.Value().(*value).id
				if act != exp {
					t.Errorf("got id=%d, expected %d", act, exp)
				}
			},
			times: 2,
		},
		{
			name:  "should replace removed items",
			size:  1,
			newFn: factory.create,
			assertFn: func(t *testing.T, n int, r *lido.Item, err error) {
				if err != nil {
					t.Errorf("got %v, expected nil", err)
				}
				defer r.Remove()

				exp := n
				act := r.Value().(*value).id
				if act != exp {
					t.Errorf("got id=%d, expected %d", act, exp)
				}
			},
			times: 2,
		},
		{
			name:  "should timeout if an item is not available",
			size:  1,
			newFn: factory.create,
			assertFn: func(t *testing.T, n int, r *lido.Item, err error) {
				switch n {
				case 0:
					if err != nil {
						t.Errorf("got %v, expected nil", err)
					}
				case 1:
					if err != lido.ErrTimeout {
						t.Errorf("got %v, expected %v", err, lido.ErrTimeout)
					}
				}
				// don't restore the item
			},
			times: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory.reset()

			p := lido.New(lido.Options{
				New:     tt.newFn,
				Size:    tt.size,
				Timeout: 100 * time.Millisecond,
			})

			for n := 0; n < tt.times; n++ {
				item, err := p.Next()
				tt.assertFn(t, n, item, err)
			}
		})
	}
}

func TestPool_Close(t *testing.T) {
	tests := []struct {
		name string
		item interface{}
		err  error
	}{
		{
			name: "should return an error if an item cannot be closed",
			item: &value{closeErr: errBasic},
			err:  errBasic,
		},
		{
			name: "should close pool items",
			item: new(value),
		},
		{
			name: "should ignore items that cannot be closed",
			item: "item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lido.New(lido.Options{
				New: func() (interface{}, error) {
					return tt.item, nil
				},
				Size:    1,
				Timeout: 100 * time.Millisecond,
			})

			func() {
				item, err := p.Next()
				if err != nil {
					t.Errorf("got %v, expected nil", err)
				}
				defer item.Restore()
			}()

			err := p.Close()
			if err != tt.err {
				t.Errorf("got %v, expected %v", err, tt.err)
			}

			if fi, ok := tt.item.(*value); ok && !fi.closed {
				t.Errorf("got closed=%v, expected true", fi.closed)
			}
		})
	}
}

func TestPoolParallel(t *testing.T) {
	t.Run("should handle parallel requests", func(t *testing.T) {
		factory := newFactory()

		p := lido.New(lido.Options{
			New:     factory.create,
			Size:    1000,
			Timeout: 10 * time.Second,
		})

		wg := new(sync.WaitGroup)
		for i := 0; i < 10000; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				item, err := p.Next()
				if err != nil {
					t.Errorf("got %v, expected nil", err)
					return
				}
				defer item.Restore()

				id := item.Value().(*value).id
				if id >= p.Size() {
					t.Errorf("got %v, expected less than 10", id)
				}

				time.Sleep(100 * time.Millisecond)
			}()
		}

		wg.Wait()
	})
}

func TestResult_Restore(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*lido.Item)
		panic bool
	}{
		{
			name: "should panic if the result was removed",
			setup: func(r *lido.Item) {
				r.Remove()
			},
			panic: true,
		},
		{
			name: "should panic if the result was already restored",
			setup: func(r *lido.Item) {
				r.Restore()
			},
			panic: true,
		},
		{
			name:  "should restore the item",
			setup: func(r *lido.Item) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lido.New(lido.Options{
				New: func() (interface{}, error) {
					return "item", nil
				},
			})

			item, err := p.Next()
			if err != nil {
				t.Errorf("got %v, expected nil", err)
			}

			tt.setup(item)

			defer func() {
				r := recover()
				if r != nil && !tt.panic {
					t.Errorf("got %v, expected nil", r)
				}
				if r == nil && tt.panic {
					t.Error("got nil, expected panic")
				}
			}()

			item.Restore()
		})
	}
}

func TestResult_Remove(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*lido.Item)
		panic bool
	}{
		{
			name: "should panic if the result was restored",
			setup: func(r *lido.Item) {
				r.Restore()
			},
			panic: true,
		},
		{
			name: "should panic if the result was already removed",
			setup: func(r *lido.Item) {
				r.Remove()
			},
			panic: true,
		},
		{
			name:  "should remove the item",
			setup: func(r *lido.Item) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lido.New(lido.Options{
				New: func() (interface{}, error) {
					return "item", nil
				},
			})

			item, err := p.Next()
			if err != nil {
				t.Errorf("got %v, expected nil", err)
			}

			tt.setup(item)

			defer func() {
				r := recover()
				if r != nil && !tt.panic {
					t.Errorf("got %v, expected nil", r)
				}
				if r == nil && tt.panic {
					t.Error("got nil, expected panic")
				}
			}()

			item.Remove()
		})
	}
}

type (
	factory struct {
		index int
		mu    *sync.Mutex
	}

	value struct {
		id       int
		closeErr error
		closed   bool
	}
)

var errBasic = errors.New("error")

func newFactory() *factory {
	return &factory{
		mu: new(sync.Mutex),
	}
}

func (f *factory) create() (interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	i := &value{id: f.index}
	f.index++
	return i, nil
}

func (f *factory) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.index = 0
}

func (v *value) Close() error {
	v.closed = true
	return v.closeErr
}
