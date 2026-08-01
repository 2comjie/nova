package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Value interface {
	Bool() (bool, error)
	Int() (int64, error)
	Float() (float64, error)
	String() (string, error)
	Duration() (time.Duration, error)
	Slice() ([]Value, error)
	Map() (map[string]Value, error)
	Scan(any) error
	Load() any
	Store(any)
}

type atomicValue struct {
	value atomic.Pointer[valueBox]
}

type valueBox struct {
	value any
}

func (v *atomicValue) Load() any {
	box := v.value.Load()
	if box == nil {
		return nil
	}
	return box.value
}

func (v *atomicValue) Store(value any) {
	v.value.Store(&valueBox{value: value})
}

func (v *atomicValue) Bool() (bool, error) {
	switch value := v.Load().(type) {
	case bool:
		return value, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strconv.ParseBool(fmt.Sprint(value))
	case string:
		return strconv.ParseBool(value)
	default:
		return false, v.typeError()
	}
}

func (v *atomicValue) Int() (int64, error) {
	switch value := v.Load().(type) {
	case int:
		return int64(value), nil
	case int8:
		return int64(value), nil
	case int16:
		return int64(value), nil
	case int32:
		return int64(value), nil
	case int64:
		return value, nil
	case uint:
		return int64(value), nil
	case uint8:
		return int64(value), nil
	case uint16:
		return int64(value), nil
	case uint32:
		return int64(value), nil
	case uint64:
		return int64(value), nil
	case float32:
		return int64(value), nil
	case float64:
		return int64(value), nil
	case string:
		return strconv.ParseInt(value, 10, 64)
	default:
		return 0, v.typeError()
	}
}

func (v *atomicValue) Float() (float64, error) {
	switch value := v.Load().(type) {
	case int:
		return float64(value), nil
	case int8:
		return float64(value), nil
	case int16:
		return float64(value), nil
	case int32:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case uint:
		return float64(value), nil
	case uint8:
		return float64(value), nil
	case uint16:
		return float64(value), nil
	case uint32:
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case float32:
		return float64(value), nil
	case float64:
		return value, nil
	case string:
		return strconv.ParseFloat(value, 64)
	default:
		return 0, v.typeError()
	}
}

func (v *atomicValue) String() (string, error) {
	switch value := v.Load().(type) {
	case string:
		return value, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(value), nil
	case []byte:
		return string(value), nil
	case fmt.Stringer:
		return value.String(), nil
	default:
		return "", v.typeError()
	}
}

func (v *atomicValue) Duration() (time.Duration, error) {
	value, err := v.Int()
	return time.Duration(value), err
}

func (v *atomicValue) Slice() ([]Value, error) {
	items, ok := v.Load().([]any)
	if !ok {
		return nil, v.typeError()
	}
	values := make([]Value, len(items))
	for i, item := range items {
		value := &atomicValue{}
		value.Store(clone(item))
		values[i] = value
	}
	return values, nil
}

func (v *atomicValue) Map() (map[string]Value, error) {
	object, ok := v.Load().(map[string]any)
	if !ok {
		return nil, v.typeError()
	}
	values := make(map[string]Value, len(object))
	for key, item := range object {
		value := &atomicValue{}
		value.Store(clone(item))
		values[key] = value
	}
	return values, nil
}

func (v *atomicValue) Scan(target any) error {
	data, err := json.Marshal(v.Load())
	if err != nil {
		return err
	}
	if message, ok := target.(proto.Message); ok {
		return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, message)
	}
	return json.Unmarshal(data, target)
}

func (v *atomicValue) typeError() error {
	return fmt.Errorf("config: cannot convert %v", reflect.TypeOf(v.Load()))
}
