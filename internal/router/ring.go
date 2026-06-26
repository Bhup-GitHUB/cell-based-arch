package router

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"
)

func fnv32a(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	b := h.Sum(nil)
	return binary.BigEndian.Uint32(b)
}

func Pick(customerID string, cells []Cell) (Cell, error) {
	if len(cells) == 0 {
		return Cell{}, fmt.Errorf("no cells registered")
	}

	sorted := make([]Cell, len(cells))
	copy(sorted, cells)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	idx := fnv32a(customerID) % uint32(len(sorted))
	return sorted[idx], nil
}
