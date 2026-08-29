package diff

import (
	"encoding/binary"
	"math"
	"reflect"

	"github.com/2comjie/nova/generic"
)

func beginValue(data []byte) ([]byte, int) {
	lengthIndex := len(data)
	return append(data, 0), lengthIndex
}

func endValue(data []byte, lengthIndex int) []byte {
	size := len(data) - lengthIndex - 1
	if size < 128 {
		data[lengthIndex] = byte(size)
		return data
	}

	var lengthData [10]byte
	length := binary.PutUvarint(lengthData[:], uint64(size))
	oldLength := len(data)
	data = append(data, lengthData[1:length]...)
	copy(data[lengthIndex+length:], data[lengthIndex+1:oldLength])
	copy(data[lengthIndex:lengthIndex+length], lengthData[:length])
	return data
}

func beginField(data []byte, diffIndex uint32) ([]byte, int) {
	data = binary.AppendUvarint(data, uint64(diffIndex))
	return beginValue(data)
}

func appendPrimitive(data []byte, value any) []byte {
	reflectValue := reflect.ValueOf(value)
	switch reflectValue.Kind() {
	case reflect.Bool:
		if reflectValue.Bool() {
			return append(data, 1)
		}
		return append(data, 0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return binary.AppendVarint(data, reflectValue.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return binary.AppendUvarint(data, reflectValue.Uint())
	case reflect.Float32:
		return binary.LittleEndian.AppendUint32(data, math.Float32bits(float32(reflectValue.Float())))
	case reflect.Float64:
		return binary.LittleEndian.AppendUint64(data, math.Float64bits(reflectValue.Float()))
	case reflect.Complex64:
		complexValue := complex64(reflectValue.Complex())
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(real(complexValue)))
		return binary.LittleEndian.AppendUint32(data, math.Float32bits(imag(complexValue)))
	case reflect.Complex128:
		complexValue := reflectValue.Complex()
		data = binary.LittleEndian.AppendUint64(data, math.Float64bits(real(complexValue)))
		return binary.LittleEndian.AppendUint64(data, math.Float64bits(imag(complexValue)))
	case reflect.String:
		return append(data, reflectValue.String()...)
	default:
		panic("diff: 不支持的基础类型")
	}
}

func DecodePrimitive[T generic.Primitive](data []byte) (T, error) {
	var value T
	reflectValue := reflect.ValueOf(&value).Elem()

	switch reflectValue.Kind() {
	case reflect.Bool:
		if len(data) != 1 || data[0] > 1 {
			return value, ErrInvalidData
		}
		reflectValue.SetBool(data[0] == 1)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		decoded, size := binary.Varint(data)
		if size <= 0 || size != len(data) || reflectValue.OverflowInt(decoded) {
			return value, ErrInvalidData
		}
		reflectValue.SetInt(decoded)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		decoded, size := binary.Uvarint(data)
		if size <= 0 || size != len(data) || reflectValue.OverflowUint(decoded) {
			return value, ErrInvalidData
		}
		reflectValue.SetUint(decoded)

	case reflect.Float32:
		if len(data) != 4 {
			return value, ErrInvalidData
		}
		reflectValue.SetFloat(float64(math.Float32frombits(binary.LittleEndian.Uint32(data))))

	case reflect.Float64:
		if len(data) != 8 {
			return value, ErrInvalidData
		}
		reflectValue.SetFloat(math.Float64frombits(binary.LittleEndian.Uint64(data)))

	case reflect.Complex64:
		if len(data) != 8 {
			return value, ErrInvalidData
		}
		realValue := math.Float32frombits(binary.LittleEndian.Uint32(data))
		imagValue := math.Float32frombits(binary.LittleEndian.Uint32(data[4:]))
		reflectValue.SetComplex(complex128(complex(realValue, imagValue)))

	case reflect.Complex128:
		if len(data) != 16 {
			return value, ErrInvalidData
		}
		realValue := math.Float64frombits(binary.LittleEndian.Uint64(data))
		imagValue := math.Float64frombits(binary.LittleEndian.Uint64(data[8:]))
		reflectValue.SetComplex(complex(realValue, imagValue))

	case reflect.String:
		reflectValue.SetString(string(data))

	default:
		return value, ErrInvalidData
	}
	return value, nil
}
