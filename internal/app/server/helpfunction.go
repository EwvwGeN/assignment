package server

import (
	"sort"
)

func Difference[T any](a, b []T) (d, c []T) {
	m := make(map[interface{}]int)

	for index, item := range a {
		m[item] = index
	}

	var adel = []int{}
	var bdel = []int{}

	for index2, item := range b {
		if index1, ok := m[item]; ok {
			adel = append(adel, index1)
			bdel = append(bdel, index2)
		}
	}
	sort.Slice(adel, func(i, j int) bool {
		return adel[i] < adel[j]
	})
	for index, sourceIndexA := range adel {
		sourceIndexB := bdel[index]
		a = append(a[:sourceIndexA-index], a[sourceIndexA-index+1:]...)
		b = append(b[:sourceIndexB-index], b[sourceIndexB-index+1:]...)
	}
	return a, b
}
