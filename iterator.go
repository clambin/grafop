package main

import (
	"iter"
	"sync/atomic"
)

// IteratorWithErrorFunc is an iterator function, where yield is the function to return values to the caller,
// and iterator is the calling iterator itself, to be used by the iterator function to set an encountered error.
type IteratorWithErrorFunc[T any] func(yield func(T) bool, iterator *IteratorWithError[T])

// IteratorWithError is a convenience iterator that can return an error.
type IteratorWithError[T any] struct {
	err atomic.Value
	f   IteratorWithErrorFunc[T]
}

// NewIteratorWithError returns a new IteratorWithError for the provided iterator function
func NewIteratorWithError[T any](f IteratorWithErrorFunc[T]) *IteratorWithError[T] {
	return &IteratorWithError[T]{
		f: f,
	}
}

// Error returns the error encountered by the iterator, if any.
func (i *IteratorWithError[T]) Error() error {
	err, _ := i.err.Load().(error)
	return err
}

// SetError sets the error encountered by the iterator.
func (i *IteratorWithError[T]) SetError(err error) {
	i.err.Store(err)
}

// All returns an iterator for all values.
func (i *IteratorWithError[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		i.f(yield, i)
	}
}
