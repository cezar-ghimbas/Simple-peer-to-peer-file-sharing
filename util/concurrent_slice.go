package util

import (
	"math"
	"math/rand"
	"sync"
)

type ConcurrentSlice struct {
	data []interface{}
	sync.RWMutex
}

func NewConcurrentSlice() *ConcurrentSlice {
	var concurrentSlice ConcurrentSlice
	concurrentSlice.data = make([]interface{}, 0)

	return &concurrentSlice
}

func (m *ConcurrentSlice) Append(value interface{}) {
	m.Lock()
	defer m.Unlock()

	m.data = append(m.data, value)
}

func (m *ConcurrentSlice) Set(index int, value interface{}) {
	m.Lock()
	defer m.Unlock()

	m.data[index] = value
}

func (m *ConcurrentSlice) Delete(index int) {
	m.Lock()
	defer m.Unlock()

	m.data = deleteHelper(m.data, index)
}

func (m *ConcurrentSlice) DeleteValue(value interface{}) {
	m.Lock()
	defer m.Unlock()

	for crntIndex, crntValue := range m.data {
		if crntValue == value {
			m.data = deleteHelper(m.data, crntIndex)
			break
		}
	}
}

func (m *ConcurrentSlice) Get(index int) interface{} {
	m.Lock()
	defer m.Unlock()

	return m.data[index]
}

func (m *ConcurrentSlice) Len() int {
	m.Lock()
	defer m.Unlock()

	return len(m.data)
}

func (m *ConcurrentSlice) ApplyOperation(operation func(interface{}, interface{})) {
	m.Lock()
	defer m.Unlock()

	for index, value := range m.data {
		operation(index, value)
	}
}

func (m *ConcurrentSlice) GetRandomValues(num int) []interface{} {
	m.Lock()
	defer m.Unlock()

	length := len(m.data)
	num = int(math.Min(float64(length), float64(num)))

	removeFromSlice := func(slice []int, index int) []int {
		return append(slice[:index], slice[index+1:]...)
	}

	var indexList []int
	for i := 0; i < length; i++ {
		indexList = append(indexList, i)
	}

	var result []interface{}
	for i := 0; i < num; i++ {
		randomIndex := rand.Intn(len(indexList))
		result = append(result, m.data[indexList[randomIndex]])
		indexList = removeFromSlice(indexList, randomIndex)
	}

	return result
}

func deleteHelper(data []interface{}, index int) []interface{} {
	return append(data[:index], data[index+1:]...)
}
