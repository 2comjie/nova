package diff

import (
	"bytes"
	"errors"
	"testing"
)

func TestValueReader(t *testing.T) {
	writer := NewWriter(nil)
	writer.AppendBool(true)
	writer.AppendEnum(-1)
	writer.AppendInt32(-2)
	writer.AppendInt64(-3)
	writer.AppendSint32(-4)
	writer.AppendSint64(-5)
	writer.AppendUint32(6)
	writer.AppendUint64(7)
	writer.AppendFixed32(8)
	writer.AppendSfixed32(-9)
	writer.AppendFloat32(1.5)
	writer.AppendFixed64(10)
	writer.AppendSfixed64(-11)
	writer.AppendFloat64(2.5)
	writer.AppendString("nova")
	writer.AppendBytes([]byte{12, 13})

	reader := NewValueReader(writer.Data())
	if !reader.Bool() {
		t.Fatal("expected true")
	}
	if value := reader.Enum(); value != -1 {
		t.Fatalf("expected -1, got %d", value)
	}
	if value := reader.Int32(); value != -2 {
		t.Fatalf("expected -2, got %d", value)
	}
	if value := reader.Int64(); value != -3 {
		t.Fatalf("expected -3, got %d", value)
	}
	if value := reader.Sint32(); value != -4 {
		t.Fatalf("expected -4, got %d", value)
	}
	if value := reader.Sint64(); value != -5 {
		t.Fatalf("expected -5, got %d", value)
	}
	if value := reader.Uint32(); value != 6 {
		t.Fatalf("expected 6, got %d", value)
	}
	if value := reader.Uint64(); value != 7 {
		t.Fatalf("expected 7, got %d", value)
	}
	if value := reader.Fixed32(); value != 8 {
		t.Fatalf("expected 8, got %d", value)
	}
	if value := reader.Sfixed32(); value != -9 {
		t.Fatalf("expected -9, got %d", value)
	}
	if value := reader.Float32(); value != 1.5 {
		t.Fatalf("expected 1.5, got %f", value)
	}
	if value := reader.Fixed64(); value != 10 {
		t.Fatalf("expected 10, got %d", value)
	}
	if value := reader.Sfixed64(); value != -11 {
		t.Fatalf("expected -11, got %d", value)
	}
	if value := reader.Float64(); value != 2.5 {
		t.Fatalf("expected 2.5, got %f", value)
	}
	if value := reader.String(); value != "nova" {
		t.Fatalf("expected nova, got %q", value)
	}
	if value := reader.Bytes(); !bytes.Equal(value, []byte{12, 13}) {
		t.Fatalf("expected [12 13], got %v", value)
	}
	if !reader.Done() || reader.Err() != nil {
		t.Fatalf("expected end, got done=%v err=%v", reader.Done(), reader.Err())
	}
}

func TestValueReaderRemaining(t *testing.T) {
	reader := NewValueReader([]byte{1, 2, 3})
	if value := reader.Uint32(); value != 1 {
		t.Fatalf("expected 1, got %d", value)
	}
	if value := reader.Remaining(); !bytes.Equal(value, []byte{2, 3}) {
		t.Fatalf("expected [2 3], got %v", value)
	}
	if !reader.Done() || reader.Err() != nil {
		t.Fatalf("expected end, got done=%v err=%v", reader.Done(), reader.Err())
	}
}

func TestValueReaderInvalidData(t *testing.T) {
	tests := []struct {
		data []byte
		read func(*ValueReader)
	}{
		{data: []byte{0x80}, read: func(reader *ValueReader) { reader.Uint64() }},
		{data: []byte{1, 2, 3}, read: func(reader *ValueReader) { reader.Fixed32() }},
		{data: []byte{1, 2, 3}, read: func(reader *ValueReader) { reader.Fixed64() }},
		{data: []byte{4, 1, 2, 3}, read: func(reader *ValueReader) { reader.Bytes() }},
	}

	for _, test := range tests {
		reader := NewValueReader(test.data)
		test.read(reader)
		if !reader.Done() || !errors.Is(reader.Err(), ErrInvalidData) {
			t.Fatalf("data %v: expected ErrInvalidData, got done=%v err=%v", test.data, reader.Done(), reader.Err())
		}
	}
}

func TestDecodeValue(t *testing.T) {
	writer := NewWriter(nil)
	writer.AppendSint32(-1)
	if value := DecodeSint32(writer.Data()); value != -1 {
		t.Fatalf("expected -1, got %d", value)
	}

	writer = NewWriter(nil)
	writer.AppendFloat64(1.5)
	if value := DecodeFloat64(writer.Data()); value != 1.5 {
		t.Fatalf("expected 1.5, got %f", value)
	}

	data := []byte("nova")
	if value := DecodeString(data); value != "nova" {
		t.Fatalf("expected nova, got %q", value)
	}
	value := DecodeBytes(data)
	data[0] = 'N'
	if string(value) != "nova" {
		t.Fatalf("DecodeBytes must clone data, got %q", value)
	}
}
