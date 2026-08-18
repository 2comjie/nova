package diff

import (
	"bytes"
	"testing"
)

func TestWriter(t *testing.T) {
	tests := []struct {
		name     string
		write    func(*Writer)
		expected []byte
	}{
		{name: "bool", write: func(writer *Writer) { writer.Bool(1, true) }, expected: []byte{33, 1}},
		{name: "enum", write: func(writer *Writer) { writer.Enum(1, 2) }, expected: []byte{33, 2}},
		{name: "int32", write: func(writer *Writer) { writer.Int32(4, 127) }, expected: []byte{129, 1, 127}},
		{name: "int64", write: func(writer *Writer) { writer.Int64(1, -1) }, expected: []byte{33, 255, 255, 255, 255, 255, 255, 255, 255, 255, 1}},
		{name: "sint32", write: func(writer *Writer) { writer.Sint32(1, -1) }, expected: []byte{33, 1}},
		{name: "sint64", write: func(writer *Writer) { writer.Sint64(1, -1) }, expected: []byte{33, 1}},
		{name: "uint32", write: func(writer *Writer) { writer.Uint32(1, 128) }, expected: []byte{33, 128, 1}},
		{name: "uint64", write: func(writer *Writer) { writer.Uint64(1, 128) }, expected: []byte{33, 128, 1}},
		{name: "fixed32", write: func(writer *Writer) { writer.Fixed32(1, 0x12345678) }, expected: []byte{34, 0x78, 0x56, 0x34, 0x12}},
		{name: "sfixed32", write: func(writer *Writer) { writer.Sfixed32(1, -1) }, expected: []byte{34, 255, 255, 255, 255}},
		{name: "float32", write: func(writer *Writer) { writer.Float32(1, 1.5) }, expected: []byte{34, 0, 0, 192, 63}},
		{name: "fixed64", write: func(writer *Writer) { writer.Fixed64(1, 0x123456789abcdef0) }, expected: []byte{35, 0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12}},
		{name: "sfixed64", write: func(writer *Writer) { writer.Sfixed64(1, -1) }, expected: []byte{35, 255, 255, 255, 255, 255, 255, 255, 255}},
		{name: "float64", write: func(writer *Writer) { writer.Float64(1, 1.5) }, expected: []byte{35, 0, 0, 0, 0, 0, 0, 248, 63}},
		{name: "string", write: func(writer *Writer) { writer.String(1, "abc") }, expected: []byte{36, 3, 'a', 'b', 'c'}},
		{name: "bytes", write: func(writer *Writer) { writer.Bytes(1, []byte{1, 2, 3}) }, expected: []byte{36, 3, 1, 2, 3}},
		{name: "clear", write: func(writer *Writer) { writer.Clear(1) }, expected: []byte{37}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := NewWriter(nil)
			test.write(writer)
			if !bytes.Equal(writer.Data(), test.expected) {
				t.Fatalf("expected %v, got %v", test.expected, writer.Data())
			}
		})
	}
}

func TestWriterAppendsToBuffer(t *testing.T) {
	writer := NewWriter([]byte{1, 2})
	writer.Bool(1, false)

	expected := []byte{1, 2, 33, 0}
	if !bytes.Equal(writer.Data(), expected) {
		t.Fatalf("expected %v, got %v", expected, writer.Data())
	}
}

func TestWriterBlock(t *testing.T) {
	writer := NewWriter(nil)
	writer.Patch(1, func(writer *Writer) {
		writer.Int32(2, 10)
	})

	expected := []byte{38, 2, 65, 10}
	if !bytes.Equal(writer.Data(), expected) {
		t.Fatalf("expected %v, got %v", expected, writer.Data())
	}
}

func TestWriterLargeBlock(t *testing.T) {
	writer := NewWriter(nil)
	writer.Replace(1, func(writer *Writer) {
		writer.AppendRaw(make([]byte, 130))
	})

	data := writer.Data()
	if len(data) != 133 || data[0] != 39 || data[1] != 130 || data[2] != 1 {
		t.Fatalf("unexpected block encoding %v", data[:3])
	}
}

func TestWriterListAndMap(t *testing.T) {
	writer := NewWriter(nil)

	writer.ListAppend(3, func(writer *Writer) {
		writer.AppendInt32(150)
	})
	writer.MapPatch(4, func(writer *Writer) {
		writer.AppendUint64(3)
	}, func(writer *Writer) {
		writer.Int64(1, 100)
	})

	expected := []byte{104, 2, 150, 1, 144, 1, 3, 3, 33, 100}
	if !bytes.Equal(writer.Data(), expected) {
		t.Fatalf("expected %v, got %v", expected, writer.Data())
	}
}

func TestWriterOperations(t *testing.T) {
	tests := []struct {
		name     string
		write    func(*Writer)
		expected []byte
	}{
		{name: "patch", write: func(writer *Writer) {
			writer.Patch(1, func(writer *Writer) {
				writer.Int32(1, 2)
			})
		}, expected: []byte{38, 2, 33, 2}},
		{name: "replace", write: func(writer *Writer) {
			writer.Replace(1, func(writer *Writer) {
				writer.AppendRaw([]byte{2})
			})
		}, expected: []byte{39, 1, 2}},
		{name: "list append", write: func(writer *Writer) {
			writer.ListAppend(1, func(writer *Writer) {
				writer.AppendInt32(2)
			})
		}, expected: []byte{40, 1, 2}},
		{name: "list insert", write: func(writer *Writer) {
			writer.ListInsert(1, 2, func(writer *Writer) {
				writer.AppendInt32(3)
			})
		}, expected: []byte{41, 2, 2, 3}},
		{name: "list set", write: func(writer *Writer) {
			writer.ListSet(1, 2, func(writer *Writer) {
				writer.AppendInt32(3)
			})
		}, expected: []byte{42, 2, 2, 3}},
		{name: "list delete", write: func(writer *Writer) {
			writer.ListDelete(1, 2)
		}, expected: []byte{43, 1, 2}},
		{name: "list move", write: func(writer *Writer) {
			writer.ListMove(1, 2, 3)
		}, expected: []byte{44, 2, 2, 3}},
		{name: "list patch", write: func(writer *Writer) {
			writer.ListPatch(1, 2, func(writer *Writer) {
				writer.Int32(1, 3)
			})
		}, expected: []byte{45, 3, 2, 33, 3}},
		{name: "map put", write: func(writer *Writer) {
			writer.MapPut(1, func(writer *Writer) {
				writer.AppendInt32(2)
			}, func(writer *Writer) {
				writer.AppendInt32(3)
			})
		}, expected: []byte{46, 2, 2, 3}},
		{name: "map delete", write: func(writer *Writer) {
			writer.MapDelete(1, func(writer *Writer) {
				writer.AppendInt32(2)
			})
		}, expected: []byte{47, 1, 2}},
		{name: "map patch", write: func(writer *Writer) {
			writer.MapPatch(1, func(writer *Writer) {
				writer.AppendInt32(2)
			}, func(writer *Writer) {
				writer.Int32(1, 3)
			})
		}, expected: []byte{48, 3, 2, 33, 3}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := NewWriter(nil)
			test.write(writer)
			if !bytes.Equal(writer.Data(), test.expected) {
				t.Fatalf("expected %v, got %v", test.expected, writer.Data())
			}
		})
	}
}
