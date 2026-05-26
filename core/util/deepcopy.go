package util

import (
	"reflect"
	"time"
)

func Clone[T any](v T) T {
	dst := reflect.New(reflect.TypeOf(v)).Elem()
	deepCopy(dst, reflect.ValueOf(v))
	return dst.Interface().(T)
}

func deepCopy(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Interface:
		if src.IsNil() {
			return
		}
		value := src.Elem()
		newValue := reflect.New(value.Type()).Elem()
		deepCopy(newValue, value)
		dst.Set(newValue)

	case reflect.Ptr:
		value := src.Elem()
		if !value.IsValid() {
			return
		}
		dst.Set(reflect.New(value.Type()))
		deepCopy(dst.Elem(), value)

	case reflect.Map:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeMapWithSize(src.Type(), src.Len()))
		for _, key := range src.MapKeys() {
			value := src.MapIndex(key)
			newValue := reflect.New(value.Type()).Elem()
			deepCopy(newValue, value)
			newKey := reflect.New(key.Type()).Elem()
			deepCopy(newKey, key)
			dst.SetMapIndex(newKey, newValue)
		}

	case reflect.Slice:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeSlice(src.Type(), src.Len(), src.Cap()))
		for i := 0; i < src.Len(); i++ {
			deepCopy(dst.Index(i), src.Index(i))
		}

	case reflect.Struct:
		if t, ok := src.Interface().(time.Time); ok {
			dst.Set(reflect.ValueOf(t))
			return
		}
		for i := 0; i < src.NumField(); i++ {
			if src.Type().Field(i).PkgPath != "" {
				continue
			}
			deepCopy(dst.Field(i), src.Field(i))
		}

	case reflect.Array:
		for i := 0; i < src.Len(); i++ {
			deepCopy(dst.Index(i), src.Index(i))
		}

	default:
		dst.Set(src)
	}
}
