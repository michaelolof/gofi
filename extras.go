package gofi

import (
	"fmt"
	"iter"
	"strings"
)

type kv struct {
	key string
	val any
}

type gofiStore struct {
	items []kv
}

type ReadOnlyStore interface {
	// Checks whether a value exists in the global store
	Has(key string) bool
	// Returns the value set in the global store using the key passed. Returns false if the value isn't found
	Get(key string) (any, bool)
	// Returns the value set in the global store using the key passed. Panics if the value isn't found
	TryGet(key string) any
	// All returns an iterator over all keys and values in the store
	All() iter.Seq2[string, any]
}

type GofiStore interface {
	ReadOnlyStore
	// Sets a value to the global store
	Set(key string, val any)
}

func NewGlobalStore() *gofiStore {
	return &gofiStore{items: make([]kv, 0, 8)}
}

func NewDataStore() *gofiStore {
	return &gofiStore{items: make([]kv, 0, 4)}
}

func (g *gofiStore) Has(key string) bool {
	for i := range g.items {
		if g.items[i].key == key {
			return true
		}
	}
	return false
}

func (g *gofiStore) Set(key string, val any) {
	for i := range g.items {
		if g.items[i].key == key {
			g.items[i].val = val
			return
		}
	}
	g.items = append(g.items, kv{key: key, val: val})
}

func (g *gofiStore) Get(key string) (any, bool) {
	for i := range g.items {
		if g.items[i].key == key {
			return g.items[i].val, true
		}
	}
	return nil, false
}

func (g *gofiStore) TryGet(key string) any {
	for i := range g.items {
		if g.items[i].key == key {
			return g.items[i].val
		}
	}
	panic(fmt.Sprintf("global value with key %s doesn't exist on context object", key))
}

func (g *gofiStore) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for _, item := range g.items {
			if !yield(item.key, item.val) {
				return
			}
		}
	}
}

type metaMap map[string]map[string]any

type contextMeta struct {
	c *context
}

type ContextMeta interface {
	This() (any, bool)
}

type RouterMeta interface {
	Route(path, method string) (any, bool)
	TryRoute(path, method string) any
	All() map[string]map[string]any
	AllSeq() iter.Seq[MetaMapInfo]
	Filter(fn func(path, method string) bool) map[string]map[string]any
	FilterAsSlice(fn func(path, method string) bool) []MetaMapInfo
	FilterSeq(fn func(path, method string) bool) iter.Seq[MetaMapInfo]
}

// Gets current meta for the current url and true if found. Returns false if not found
func (m *contextMeta) This() (any, bool) {
	v, f := m.c.routeMeta[m.c.opts.Pattern][strings.ToLower(m.c.opts.Method)]
	return v, f
}

func (m metaMap) Route(path, method string) (any, bool) {
	v, ok := m[path][strings.ToLower(method)]
	return v, ok
}

func (m metaMap) TryRoute(path, method string) any {
	if v, ok := m[path][strings.ToLower(method)]; !ok {
		panic(fmt.Sprintf("Meta information doesn't exist for the given path [%s %s]", method, path))
	} else {
		return v
	}
}

func (m metaMap) All() map[string]map[string]any {
	return m
}

func (m metaMap) Filter(fn func(path, method string) bool) map[string]map[string]any {
	r := map[string]map[string]any{}
	for p, v := range m {
		for mt, vp := range v {
			if fn(p, mt) {
				temp := map[string]any{mt: vp}
				r[p] = temp
			}
		}
	}
	return r
}

type MetaMapInfo struct {
	Path      string
	Method    string
	MetaValue any
}

func (m metaMap) FilterAsSlice(fn func(path, method string) bool) []MetaMapInfo {
	r := make([]MetaMapInfo, 0, 4*len(m))

	for p, v := range m {
		for mt, vp := range v {
			if fn(p, mt) {
				r = append(r, MetaMapInfo{Path: p, Method: mt, MetaValue: vp})
			}
		}
	}

	return r
}

func (m metaMap) AllSeq() iter.Seq[MetaMapInfo] {
	return func(yield func(MetaMapInfo) bool) {
		for p, v := range m {
			for mt, vp := range v {
				if !yield(MetaMapInfo{Path: p, Method: mt, MetaValue: vp}) {
					return
				}
			}
		}
	}
}

func (m metaMap) FilterSeq(fn func(path, method string) bool) iter.Seq[MetaMapInfo] {
	return func(yield func(MetaMapInfo) bool) {
		for p, v := range m {
			for mt, vp := range v {
				if fn(p, mt) {
					if !yield(MetaMapInfo{Path: p, Method: mt, MetaValue: vp}) {
						return
					}
				}
			}
		}
	}
}
