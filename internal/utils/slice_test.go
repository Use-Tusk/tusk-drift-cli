package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBatchSlice(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := BatchSlice([]int{}, 3)
		assert.Empty(t, result)
	})

	t.Run("nil slice", func(t *testing.T) {
		var items []int
		result := BatchSlice(items, 3)
		assert.Empty(t, result)
	})

	t.Run("batch size larger than slice", func(t *testing.T) {
		items := []int{1, 2, 3}
		result := BatchSlice(items, 10)
		assert.Len(t, result, 1)
		assert.Equal(t, []int{1, 2, 3}, result[0])
	})

	t.Run("exact batch size", func(t *testing.T) {
		items := []int{1, 2, 3, 4, 5, 6}
		result := BatchSlice(items, 3)
		assert.Len(t, result, 2)
		assert.Equal(t, []int{1, 2, 3}, result[0])
		assert.Equal(t, []int{4, 5, 6}, result[1])
	})

	t.Run("partial last batch", func(t *testing.T) {
		items := []int{1, 2, 3, 4, 5}
		result := BatchSlice(items, 2)
		assert.Len(t, result, 3)
		assert.Equal(t, []int{1, 2}, result[0])
		assert.Equal(t, []int{3, 4}, result[1])
		assert.Equal(t, []int{5}, result[2])
	})

	t.Run("batch size of 1", func(t *testing.T) {
		items := []int{1, 2, 3}
		result := BatchSlice(items, 1)
		assert.Len(t, result, 3)
		assert.Equal(t, []int{1}, result[0])
		assert.Equal(t, []int{2}, result[1])
		assert.Equal(t, []int{3}, result[2])
	})

	t.Run("batch size of 0 returns nil", func(t *testing.T) {
		items := []int{1, 2, 3}
		result := BatchSlice(items, 0)
		assert.Nil(t, result)
	})

	t.Run("negative batch size returns nil", func(t *testing.T) {
		items := []int{1, 2, 3}
		result := BatchSlice(items, -1)
		assert.Nil(t, result)
	})

	t.Run("works with strings", func(t *testing.T) {
		items := []string{"a", "b", "c", "d"}
		result := BatchSlice(items, 2)
		assert.Len(t, result, 2)
		assert.Equal(t, []string{"a", "b"}, result[0])
		assert.Equal(t, []string{"c", "d"}, result[1])
	})

	t.Run("works with structs", func(t *testing.T) {
		type item struct {
			id int
		}
		items := []item{{1}, {2}, {3}}
		result := BatchSlice(items, 2)
		assert.Len(t, result, 2)
		assert.Equal(t, []item{{1}, {2}}, result[0])
		assert.Equal(t, []item{{3}}, result[1])
	})

	t.Run("works with pointers", func(t *testing.T) {
		a, b, c := 1, 2, 3
		items := []*int{&a, &b, &c}
		result := BatchSlice(items, 2)
		assert.Len(t, result, 2)
		assert.Equal(t, []*int{&a, &b}, result[0])
		assert.Equal(t, []*int{&c}, result[1])
	})
}
