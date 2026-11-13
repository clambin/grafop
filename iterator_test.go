package main

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIteratorWithError(t *testing.T) {
	tests := []struct {
		name    string
		f       IteratorWithErrorFunc[int]
		wantErr require.ErrorAssertionFunc
		want    []int
	}{
		{
			name: "no error",
			f: IteratorWithErrorFunc[int](func(yield func(int) bool, _ *IteratorWithError[int]) {
				for i := range 3 {
					yield(i)
				}
			}),
			wantErr: require.NoError,
			want:    []int{0, 1, 2},
		},
		{
			name: "error",
			f: IteratorWithErrorFunc[int](func(_ func(int) bool, iterator *IteratorWithError[int]) {
				iterator.SetError(assert.AnError)
			}),
			wantErr: require.Error,
		},
		{
			name: "error after yield",
			f: IteratorWithErrorFunc[int](func(yield func(int) bool, iterator *IteratorWithError[int]) {
				for i := range 3 {
					yield(i)
				}
				iterator.SetError(assert.AnError)
			}),
			wantErr: require.Error,
			want:    []int{0, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewIteratorWithError(tt.f)
			values := slices.Collect(i.All())
			tt.wantErr(t, i.Error())
			assert.Equal(t, tt.want, values)
		})
	}
}
