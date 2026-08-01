package etc

import (
	"os"
	"time"

	"github.com/spf13/cast"
)

func Has(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func String(key string, def ...string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	if len(def) > 0 {
		return def[0]
	}
	return ""
}

func Bool(key string, def ...bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToBool(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return false
}

func Int(key string, def ...int) int {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToInt(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Int8(key string, def ...int8) int8 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToInt8(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Int16(key string, def ...int16) int16 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToInt16(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Int32(key string, def ...int32) int32 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToInt32(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Int64(key string, def ...int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToInt64(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Uint(key string, def ...uint) uint {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToUint(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Uint8(key string, def ...uint8) uint8 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToUint8(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Uint16(key string, def ...uint16) uint16 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToUint16(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Uint32(key string, def ...uint32) uint32 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToUint32(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Uint64(key string, def ...uint64) uint64 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToUint64(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Float32(key string, def ...float32) float32 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToFloat32(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Float64(key string, def ...float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToFloat64(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func Duration(key string, def ...time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		return cast.ToDuration(value)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}
