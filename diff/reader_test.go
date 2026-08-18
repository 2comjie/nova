package diff

import (
	"bytes"
	"errors"
	"testing"
)

func TestReader(t *testing.T) {
	writer := NewWriter(nil)
	writer.Int32(1, 10)
	writer.Fixed32(2, 20)
	writer.Fixed64(3, 30)
	writer.String(4, "nova")
	writer.Clear(5)
	writer.Patch(6, func(writer *Writer) {
		writer.Int32(1, 40)
	})

	expected := []struct {
		fieldNumber uint32
		operation   Operation
		payload     []byte
	}{
		{fieldNumber: 1, operation: OperationSetVarint, payload: []byte{10}},
		{fieldNumber: 2, operation: OperationSetFixed32, payload: []byte{20, 0, 0, 0}},
		{fieldNumber: 3, operation: OperationSetFixed64, payload: []byte{30, 0, 0, 0, 0, 0, 0, 0}},
		{fieldNumber: 4, operation: OperationSetBytes, payload: []byte("nova")},
		{fieldNumber: 5, operation: OperationClear},
		{fieldNumber: 6, operation: OperationPatch, payload: []byte{33, 40}},
	}

	reader := NewReader(writer.Data())
	for index, item := range expected {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil {
			t.Fatalf("record %d: %v", index, err)
		}
		if !ok {
			t.Fatalf("record %d: unexpected end", index)
		}
		if fieldNumber != item.fieldNumber || operation != item.operation || !bytes.Equal(payload, item.payload) {
			t.Fatalf("record %d: expected field=%d operation=%d payload=%v, got field=%d operation=%d payload=%v", index, item.fieldNumber, item.operation, item.payload, fieldNumber, operation, payload)
		}
	}

	_, _, _, ok, err := reader.Next()
	if err != nil || ok {
		t.Fatalf("expected end, got ok=%v err=%v", ok, err)
	}
}

func TestReaderNestedPatch(t *testing.T) {
	writer := NewWriter(nil)
	writer.Patch(1, func(writer *Writer) {
		writer.Patch(2, func(writer *Writer) {
			writer.Int32(3, 100)
		})
	})

	reader := NewReader(writer.Data())
	fieldNumber, operation, payload, ok, err := reader.Next()
	if err != nil || !ok || fieldNumber != 1 || operation != OperationPatch {
		t.Fatalf("unexpected root field=%d operation=%d ok=%v err=%v", fieldNumber, operation, ok, err)
	}

	reader = NewReader(payload)
	fieldNumber, operation, payload, ok, err = reader.Next()
	if err != nil || !ok || fieldNumber != 2 || operation != OperationPatch {
		t.Fatalf("unexpected child field=%d operation=%d ok=%v err=%v", fieldNumber, operation, ok, err)
	}

	reader = NewReader(payload)
	fieldNumber, operation, payload, ok, err = reader.Next()
	if err != nil || !ok || fieldNumber != 3 || operation != OperationSetVarint || !bytes.Equal(payload, []byte{100}) {
		t.Fatalf("unexpected leaf field=%d operation=%d payload=%v ok=%v err=%v", fieldNumber, operation, payload, ok, err)
	}
}

func TestReaderLargeBlock(t *testing.T) {
	value := bytes.Repeat([]byte{1}, 130)
	writer := NewWriter(nil)
	writer.Replace(1, func(writer *Writer) {
		writer.AppendRaw(value)
	})

	reader := NewReader(writer.Data())
	fieldNumber, operation, payload, ok, err := reader.Next()
	if err != nil || !ok || fieldNumber != 1 || operation != OperationReplace || !bytes.Equal(payload, value) {
		t.Fatalf("unexpected field=%d operation=%d payload length=%d ok=%v err=%v", fieldNumber, operation, len(payload), ok, err)
	}
}

func TestReaderInvalidData(t *testing.T) {
	tests := [][]byte{
		{0x80},
		{1},
		{33, 0x80},
		{34, 1, 2, 3},
		{35, 1, 2, 3, 4, 5, 6, 7},
		{36, 0x80},
		{36, 2, 1},
		{49},
	}

	for _, data := range tests {
		reader := NewReader(data)
		_, _, _, _, err := reader.Next()
		if !errors.Is(err, ErrInvalidData) {
			t.Fatalf("data %v: expected ErrInvalidData, got %v", data, err)
		}
	}
}
