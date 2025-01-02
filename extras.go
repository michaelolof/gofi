package gofi

import (
	"fmt"
	"strings"
)

type gofiStore struct {
	m map[string]any
}

type ReadOnlyStore interface {
	// Checks whether a value exists in the global store
	Has(key string) bool
	// Returns the value set in the global store using the key passed. Returns false if the value isn't found
	Get(key string) (any, bool)
	// Returns the value set in the global store using the key passed. Panics if the value isn't found
	TryGet(key string) any
}

type GofiStore interface {
	ReadOnlyStore
	// Sets a value to the global store
	Set(key string, val any)
}

func NewGlobalStore() *gofiStore {
	return &gofiStore{m: map[string]any{}}
}

func NewDataStore() *gofiStore {
	return &gofiStore{m: map[string]any{}}
}

func (g *gofiStore) Has(key string) bool {
	_, found := g.m[key]
	return found
}

func (g *gofiStore) Set(key string, val any) {
	g.m[key] = val
}

func (g *gofiStore) Get(key string) (any, bool) {
	v, found := g.m[key]
	return v, found
}

func (g *gofiStore) TryGet(key string) any {
	if v, found := g.m[key]; !found {
		panic(fmt.Sprintf("global value with key %s doesn't exist on context object", key))
	} else {
		return v
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
	Filter(fn func(path, method string) bool) map[string]map[string]any
	FilterAsSlice(fn func(path, method string) bool) []MetaMapInfo
}

// Gets current meta for the current url and true if found. Returns false if not found
func (m *contextMeta) This() (any, bool) {
	v, f := m.c.routeMeta[m.c.r.URL.String()][strings.ToLower(m.c.r.Method)]
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
