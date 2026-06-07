package network

import "sync"

type connAttr struct {
	values sync.Map
}

func (a *connAttr) Get(key string) (string, bool) {
	v, ok := a.values.Load(key)
	if !ok {
		return "", false
	}
	return v.(string), true
}

func (a *connAttr) Set(key, value string) { a.values.Store(key, value) }
func (a *connAttr) Del(key string) bool   { _, ok := a.values.LoadAndDelete(key); return ok }
func (a *connAttr) Visit(fn func(key, value string) bool) {
	a.values.Range(func(k, v any) bool {
		return fn(k.(string), v.(string))
	})
}
