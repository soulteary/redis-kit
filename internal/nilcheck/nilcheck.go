// Package nilcheck reports whether an interface value has nothing behind it.
//
// It exists because redis-kit's constructors take redis.UniversalClient rather
// than *redis.Client. A concrete pointer parameter made "no client" a single
// state that `== nil` recognised. An interface parameter splits it in two: a
// nil interface, and a non-nil interface holding a nil pointer -- which is
// what a caller passes from an unassigned struct field or a constructor that
// returned early:
//
//	var rdb *redis.Client        // never assigned
//	c := cache.NewCache(rdb, "") // c.client != nil, and every call panics
//
// Every nil check in this module goes through [IsNil] so that case keeps
// returning "redis client is nil" instead of dereferencing a nil pointer.
package nilcheck

import "reflect"

// IsNil reports whether v is a nil interface or an interface holding a nil
// pointer, map, slice, channel or function.
//
// A non-pointer value -- a struct implementing the interface directly, as
// test doubles often do -- is never nil and reports false.
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
