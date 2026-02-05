package utils

// BatchSlice splits a slice into batches of the specified size.
// The last batch may contain fewer elements if the slice length
// is not evenly divisible by batchSize.
func BatchSlice[T any](items []T, batchSize int) [][]T {
	if batchSize <= 0 {
		return nil
	}
	var batches [][]T
	for i := 0; i < len(items); i += batchSize {
		end := min(i+batchSize, len(items))
		batches = append(batches, items[i:end])
	}
	return batches
}
