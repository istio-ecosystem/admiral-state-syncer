package types

import "sync"

type Map struct {
	cache map[string]string
	mutex *sync.Mutex
}

type MapOfMaps struct {
	cache map[string]*Map
	mutex *sync.RWMutex
}

type MapOfMapOfMaps struct {
	cache map[string]*MapOfMaps
	mutex *sync.RWMutex
}

func NewMap() *Map {
	n := new(Map)
	n.cache = make(map[string]string)
	n.mutex = &sync.Mutex{}
	return n
}

func NewMapOfMaps() *MapOfMaps {
	n := new(MapOfMaps)
	n.cache = make(map[string]*Map)
	n.mutex = &sync.RWMutex{}
	return n
}

func NewMapOfMapOfMaps() *MapOfMapOfMaps {
	n := new(MapOfMapOfMaps)
	n.cache = make(map[string]*MapOfMaps)
	n.mutex = &sync.RWMutex{}
	return n
}

func (s *Map) Put(key string, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[key] = value
}

func (s *Map) Get(key string) string {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return s.cache[key]
}

func (s *Map) CheckIfPresent(key string) bool {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	if _, ok := s.cache[key]; ok {
		return true
	}
	return false
}

func (s *Map) Len() int {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return len(s.cache)
}

func (s *Map) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

func (s *Map) Copy() map[string]string {
	if s != nil {
		defer s.mutex.Unlock()
		s.mutex.Lock()
		var copy = make(map[string]string)
		for k, v := range s.cache {
			copy[k] = v
		}
		return copy
	} else {
		return nil
	}
}

func (s *Map) CopyJustValues() []string {
	var copy []string
	if s != nil {
		defer s.mutex.Unlock()
		s.mutex.Lock()
		for _, v := range s.cache {
			copy = append(copy, v)
		}
	}
	return copy
}

func (s *Map) Range(fn func(k string, v string)) {
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
	s.mutex.Unlock()
}

func (s *MapOfMaps) Put(pkey string, key string, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[pkey]
	if mapVal == nil {
		mapVal = NewMap()
	}
	mapVal.Put(key, value)
	s.cache[pkey] = mapVal
}

func (s *MapOfMaps) DeleteMap(pkey string, key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapVal = s.cache[pkey]
	if mapVal == nil {
		return
	}
	mapVal.Delete(key)
	s.cache[pkey] = mapVal
}

func (s *MapOfMaps) PutMap(pkey string, inputMap *Map) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[pkey] = inputMap
}

func (s *MapOfMaps) Get(key string) *Map {
	s.mutex.Lock()
	val := s.cache[key]
	s.mutex.Unlock()
	return val
}

func (s *MapOfMaps) Delete(key string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	delete(s.cache, key)
}

func (s *MapOfMaps) Range(fn func(k string, v *Map)) {
	s.mutex.Lock()
	for k, v := range s.cache {
		fn(k, v)
	}
	s.mutex.Unlock()
}

func (s *MapOfMaps) Len() int {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	return len(s.cache)
}

func (s *MapOfMaps) GetKeys() []string {
	defer s.mutex.RUnlock()
	s.mutex.RLock()
	keys := []string{}
	for k := range s.cache {
		keys = append(keys, k)
	}
	return keys
}

func (s *MapOfMapOfMaps) Put(pkey string, skey string, key, value string) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	var mapOfMapsVal = s.cache[pkey]
	if mapOfMapsVal == nil {
		mapOfMapsVal = NewMapOfMaps()
	}
	mapOfMapsVal.Put(skey, key, value)
	s.cache[pkey] = mapOfMapsVal
}

func (s *MapOfMapOfMaps) PutMapofMaps(key string, value *MapOfMaps) {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	s.cache[key] = value
}

func (s *MapOfMapOfMaps) Get(key string) *MapOfMaps {
	s.mutex.RLock()
	val := s.cache[key]
	s.mutex.RUnlock()
	return val
}

func (s *MapOfMapOfMaps) Len() int {
	defer s.mutex.RUnlock()
	s.mutex.RLock()
	return len(s.cache)
}

func (s *Map) GetKeys() []string {
	defer s.mutex.Unlock()
	s.mutex.Lock()
	keys := make([]string, 0)
	for _, val := range s.cache {
		keys = append(keys, val)
	}
	return keys
}
