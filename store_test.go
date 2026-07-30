package gofi

import "testing"

type dbKey struct{}
type tenantKey struct{}

func TestStore_StringBackwardCompat(t *testing.T) {
	s := newGlobalStore()

	// Deprecated string API must keep working unchanged.
	s.Set("db", 42)
	if !s.Has("db") {
		t.Fatal("Has(db) = false, want true")
	}
	if v, ok := s.Get("db"); !ok || v.(int) != 42 {
		t.Fatalf("Get(db) = %v, %v; want 42, true", v, ok)
	}
	if v := s.TryGet("db"); v.(int) != 42 {
		t.Fatalf("TryGet(db) = %v; want 42", v)
	}

	// Overwrite via Set updates in place.
	s.Set("db", 7)
	if v, _ := s.Get("db"); v.(int) != 7 {
		t.Fatalf("Get(db) after overwrite = %v; want 7", v)
	}
}

func TestStore_StructKeys(t *testing.T) {
	s := newGlobalStore()

	StoreSet(s, dbKey{}, "postgres")
	StoreSet(s, tenantKey{}, 99)

	if !StoreHas(s, dbKey{}) {
		t.Fatal("StoreHas(dbKey) = false, want true")
	}
	// StoreGet returns any; asserting to the expected type is the caller's job.
	if got, ok := StoreGet(s, dbKey{}); !ok || got.(string) != "postgres" {
		t.Fatalf("StoreGet(dbKey) = %v, %v; want postgres, true", got, ok)
	}
	if got, ok := StoreGet(s, tenantKey{}); !ok || got.(int) != 99 {
		t.Fatalf("StoreGet(tenantKey) = %v, %v; want 99, true", got, ok)
	}
	if got := StoreTryGet(s, dbKey{}); got.(string) != "postgres" {
		t.Fatalf("StoreTryGet(dbKey) = %v; want postgres", got)
	}
}

func TestStore_KeysDoNotCollide(t *testing.T) {
	s := newGlobalStore()

	// Distinct named struct types must never clash, even though both are
	// zero-sized and structurally identical.
	StoreSet(s, dbKey{}, "db-val")
	StoreSet(s, tenantKey{}, "tenant-val")

	if got, _ := StoreGet(s, dbKey{}); got.(string) != "db-val" {
		t.Fatalf("dbKey = %v; want db-val", got)
	}
	if got, _ := StoreGet(s, tenantKey{}); got.(string) != "tenant-val" {
		t.Fatalf("tenantKey = %v; want tenant-val", got)
	}

	// A string key "dbKey" must not collide with the dbKey{} struct key.
	StoreSet(s, "dbKey", "string-val")
	if got, _ := StoreGet(s, dbKey{}); got.(string) != "db-val" {
		t.Fatalf("dbKey struct key corrupted by string key: %v", got)
	}
}

func TestStore_GetMissing(t *testing.T) {
	s := newGlobalStore()
	StoreSet(s, dbKey{}, "postgres")

	if v, ok := StoreGet(s, tenantKey{}); ok || v != nil {
		t.Fatalf("StoreGet for missing key = %v, %v; want nil, false", v, ok)
	}
}

func TestStore_CallerAssertionPanicsOnWrongType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("caller's type assertion on wrong-typed value did not panic")
		}
	}()
	s := newGlobalStore()
	StoreSet(s, dbKey{}, "postgres")
	v, _ := StoreGet(s, dbKey{})
	_ = v.(int) // stored value is a string; asserting int panics
}

func TestStore_TryGetPanicsWhenMissing(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("StoreTryGet on missing key did not panic")
		}
	}()
	s := newGlobalStore()
	_ = StoreTryGet(s, dbKey{})
}

func TestStore_DeprecatedAllSkipsStructKeys(t *testing.T) {
	s := newGlobalStore()
	s.Set("str", 1)
	StoreSet(s, dbKey{}, 2)

	seen := map[string]any{}
	for k, v := range s.All() {
		seen[k] = v
	}
	if len(seen) != 1 || seen["str"] != 1 {
		t.Fatalf("All() = %v; want only the string key", seen)
	}

	// StoreAll sees everything.
	count := 0
	for range StoreAll(s) {
		count++
	}
	if count != 2 {
		t.Fatalf("StoreAll count = %d; want 2", count)
	}
}

// TestStore_EmptyStructKeyZeroAlloc proves the performance claim: using an
// empty struct as a key allocates nothing when boxed into the store.
func TestStore_EmptyStructKeyZeroAlloc(t *testing.T) {
	s := newGlobalStore()
	StoreSet(s, dbKey{}, "seed") // pre-create the slot so Set overwrites in place

	allocs := testing.AllocsPerRun(1000, func() {
		StoreSet(s, dbKey{}, "value")
		_, _ = StoreGet(s, dbKey{})
	})
	if allocs != 0 {
		t.Fatalf("empty struct key caused %v allocs/op; want 0", allocs)
	}
}
