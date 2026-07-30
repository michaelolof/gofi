package gofi

import (
	"fmt"
	"iter"
	"strings"
)

type kv struct {
	key any
	val any
}

// gofiStore is a small, unsynchronized linear-scan key/value store. It backs
// both the router-level GlobalStore and the per-request DataStore.
//
// Concurrency contract — read this before writing to a GlobalStore:
//
//   - DataStore is always safe. It is owned by a single request's Context and
//     is never shared across goroutines. (If you fan work out to goroutines,
//     use Context.Copy() first, per the package docs — Copy deep-copies the
//     items slice, so each copy has its own store.)
//   - GlobalStore is NOT internally synchronized. It is a single instance
//     shared by every in-flight request. Writes (Set/StoreSet) mutate the
//     backing slice in place and may reallocate it via append, which races
//     with any concurrent read (Get/Has/StoreGet/...) under the Go race
//     detector and in practice.
//
// The store is deliberately unlocked for performance: GlobalStore exists to
// register shared plugins/config once, at setup time, before the server
// starts accepting requests — the same pattern the standard library's
// context.WithValue relies on, where safety comes from never mutating shared
// state after other goroutines can observe it, rather than from locking
// around every access.
//
// Therefore:
//
//   - DO all GlobalStore().Set(...) / StoreSet(...) calls during router setup,
//     before calling Listen/ListenTLS/Serve.
//   - DO NOT call Set/StoreSet on a GlobalStore from inside a request handler
//     or middleware, or from any goroutine that runs concurrently with a live
//     server. Doing so is a data race, not just a logical inconsistency.
//   - Reads (Get/Has/TryGet/StoreGet/...) from within handlers are safe as
//     long as all writes finished before Serve() started.
//
// If you need state that individual requests populate at runtime, use
// Context.DataStore() instead — it is scoped to one request and requires no
// synchronization.
type gofiStore struct {
	items []kv
}

type ReadOnlyStore interface {
	// Deprecated: Has only supports string keys. Use the generic [StoreHas]
	// with a comparable key (an empty struct{} type is recommended) instead.
	// This method will be removed in a future release.
	Has(key string) bool
	// Deprecated: Get only supports string keys. Use the generic [StoreGet]
	// with a comparable key (an empty struct{} type is recommended) instead.
	// This method will be removed in a future release.
	Get(key string) (any, bool)
	// Deprecated: TryGet only supports string keys. Use the generic [StoreTryGet]
	// with a comparable key (an empty struct{} type is recommended) instead.
	// This method will be removed in a future release.
	TryGet(key string) any
	// Deprecated: All only yields string keys and silently skips struct keys.
	// Use [StoreAll] to iterate over every key/value pair. This method will be
	// removed in a future release.
	All() iter.Seq2[string, any]
}

type GofiStore interface {
	ReadOnlyStore
	// Deprecated: Set only supports string keys. Use the generic [StoreSet]
	// with a comparable key (an empty struct{} type is recommended) instead.
	// This method will be removed in a future release.
	//
	// On a GlobalStore, Set is not safe to call concurrently with reads or
	// other writes — see the [gofiStore] doc comment. Call it during setup,
	// before the server starts serving requests. On a DataStore (request-
	// scoped, never shared across goroutines) this concern does not apply.
	Set(key string, val any)
}

// anyKeyStore is the unexported, comparable-key backend that powers the
// generic store helpers ([StoreSet], [StoreGet], [StoreHas], [StoreTryGet],
// [StoreAll]). Keeping it unexported means the public store interfaces gain no
// new methods, so adding struct-key support is a fully backward-compatible
// change. *gofiStore is the sole implementation.
type anyKeyStore interface {
	setAny(key any, val any)
	getAny(key any) (any, bool)
	hasAny(key any) bool
	tryGetAny(key any) any
	allAny() iter.Seq2[any, any]
}

// newGlobalStore creates the single store backing a router's GlobalStore(). It
// is intentionally unexported: the router is the sole owner of the global
// store, created once in newRouter and shared (by reference) with every
// sub-router and request Context. Keeping this internal is what makes
// "global to the router" a guarantee rather than a call-site convention —
// there is no way to mint a second, detached "global" store. It is not safe
// for concurrent writes; see the gofiStore doc comment.
func newGlobalStore() *gofiStore {
	return &gofiStore{items: make([]kv, 0, 8)}
}

// newDataStore creates the per-request data store returned by
// Context.DataStore(). It is intentionally unexported: a Context is the sole
// owner of its data store (allocated lazily on first access, and freshly for
// each Context.Copy), which is what keeps the data store scoped to a single
// request rather than to whoever calls a constructor.
func newDataStore() *gofiStore {
	return &gofiStore{items: make([]kv, 0, 4)}
}

func (g *gofiStore) Has(key string) bool { return g.hasAny(key) }

func (g *gofiStore) Set(key string, val any) { g.setAny(key, val) }

func (g *gofiStore) Get(key string) (any, bool) { return g.getAny(key) }

func (g *gofiStore) TryGet(key string) any {
	if v, ok := g.getAny(key); ok {
		return v
	}
	panic(fmt.Sprintf("global value with key %s doesn't exist on context object", key))
}

func (g *gofiStore) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for _, item := range g.items {
			// String-only for backward compatibility; struct keys are skipped.
			// Use StoreAll to iterate over every key/value pair.
			if s, ok := item.key.(string); ok {
				if !yield(s, item.val) {
					return
				}
			}
		}
	}
}

// --- any-keyed backend (unexported) -----------------------------------------

func (g *gofiStore) hasAny(key any) bool {
	for i := range g.items {
		if g.items[i].key == key {
			return true
		}
	}
	return false
}

func (g *gofiStore) setAny(key any, val any) {
	for i := range g.items {
		if g.items[i].key == key {
			g.items[i].val = val
			return
		}
	}
	g.items = append(g.items, kv{key: key, val: val})
}

func (g *gofiStore) getAny(key any) (any, bool) {
	for i := range g.items {
		if g.items[i].key == key {
			return g.items[i].val, true
		}
	}
	return nil, false
}

func (g *gofiStore) tryGetAny(key any) any {
	for i := range g.items {
		if g.items[i].key == key {
			return g.items[i].val
		}
	}
	panic(fmt.Sprintf("store value with key %v doesn't exist", key))
}

func (g *gofiStore) allAny() iter.Seq2[any, any] {
	return func(yield func(any, any) bool) {
		for _, item := range g.items {
			if !yield(item.key, item.val) {
				return
			}
		}
	}
}

// --- generic store helpers --------------------------------------------------
//
// These are the recommended way to read from and write to a store. A key may
// be any comparable value, but a distinct, empty struct{} type per key is the
// recommended convention: it is collision-proof (two different named types can
// never clash) and the fastest option (boxing a zero-sized value into an
// interface allocates nothing, and comparisons short-circuit on the type
// descriptor). Keys of non-comparable types (slices, maps, funcs) will panic,
// exactly as with the standard library's context.WithValue.
//
//	type dbKey struct{}
//	gofi.StoreSet(r.GlobalStore(), dbKey{}, myDB)
//	db, ok := gofi.StoreGet[*Database](c.GlobalStore(), dbKey{})

// StoreSet stores val under key in the given store.
//
// On a GlobalStore, StoreSet is not safe to call concurrently with reads or
// other writes on the same store — see the package-level store concurrency
// notes on [gofiStore]. Call it during router setup, before the server starts
// serving requests. On a DataStore (request-scoped, never shared across
// goroutines) this concern does not apply.
func StoreSet[K comparable](s GofiStore, key K, val any) {
	s.(anyKeyStore).setAny(key, val)
}

// StoreHas reports whether a value exists for key in the store.
func StoreHas[K comparable](s ReadOnlyStore, key K) bool {
	return s.(anyKeyStore).hasAny(key)
}

// StoreGet returns the value stored under key, asserted to T, and whether it
// was found. If the key is missing, or the stored value is not a T, it returns
// the zero value of T and false.
func StoreGet[T any, K comparable](s ReadOnlyStore, key K) (T, bool) {
	v, ok := s.(anyKeyStore).getAny(key)
	if !ok {
		var zero T
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// StoreTryGet returns the value stored under key asserted to T. It panics if
// the key is missing or the stored value is not a T.
func StoreTryGet[T any, K comparable](s ReadOnlyStore, key K) T {
	return s.(anyKeyStore).tryGetAny(key).(T)
}

// StoreAll returns an iterator over every key/value pair in the store,
// including struct keys (unlike the deprecated ReadOnlyStore.All).
func StoreAll(s ReadOnlyStore) iter.Seq2[any, any] {
	return s.(anyKeyStore).allAny()
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
