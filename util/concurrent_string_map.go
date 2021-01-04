package util

import (
	"sync"
)

type ConcurrentStringMap struct {
	data map[string]interface{}
	sync.RWMutex
}

func NewConcurrentStringMap() *ConcurrentStringMap {
	var concurrentStringMap ConcurrentStringMap
	concurrentStringMap.data = make(map[string]interface{})

	return &concurrentStringMap
}

func (m *ConcurrentStringMap) Set(key string, value interface{}) {
	m.Lock()
	defer m.Unlock()

	m.data[key] = value
}

func (m *ConcurrentStringMap) AddUnique(key string, value interface{}) bool {
	m.Lock()
	defer m.Unlock()

	_, keyFound := m.data[key]
	if keyFound == false {
		m.data[key] = value
	}

	return keyFound
}

func (m *ConcurrentStringMap) Delete(key string) {
	m.Lock()
	defer m.Unlock()

	delete(m.data, key)
}

func (m *ConcurrentStringMap) Get(key string) (interface{}, bool) {
	m.Lock()
	defer m.Unlock()

	value, keyFound := m.data[key]

	return value, keyFound
}

func (m *ConcurrentStringMap) Len() int {
	m.Lock()
	defer m.Unlock()

	return len(m.data)
}

func (m *ConcurrentStringMap) HasKey(key string) bool {
	m.Lock()
	defer m.Unlock()

	_, keyFound := m.data[key]

	return keyFound
}

func (m *ConcurrentStringMap) ApplyOperation(operation func(interface{}, interface{})) {
	m.Lock()
	defer m.Unlock()

	for key, value := range m.data {
		operation(key, value)
	}
}
