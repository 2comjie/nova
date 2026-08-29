package diff_test

import (
	"reflect"
	"testing"

	"github.com/2comjie/nova/diff"
)

type Player struct {
	diff.Object
	Bag  diff.Pointer[*Bag]     `diff:"1"`
	Age  diff.Primitive[int32]  `diff:"2"`
	Name diff.Primitive[string] `diff:"3"`
}

type Bag struct {
	diff.Object
	Scores diff.PrimitiveMap[int32, int32] `diff:"1"`
	Hold   diff.Pointer[*Item]             `diff:"2"`
}

type Item struct {
	diff.Object
	Id    diff.Primitive[int32] `diff:"1"`
	Count diff.Primitive[int32] `diff:"2"`
}

type Collections struct {
	diff.Object
	Scores  diff.PrimitiveMap[int32, int32] `diff:"1"`
	Numbers diff.PrimitiveSlice[int32]      `diff:"2"`
	Items   diff.PointerMap[int32, *Item]   `diff:"3"`
	Order   diff.PointerSlice[*Item]        `diff:"4"`
}

func (p *Player) InitLink(writer *diff.Writer) {
	p.Object.Init(writer)
	p.Bag.Init(&p.Object, 1)
	p.Age.Init(&p.Object, 2)
	p.Name.Init(&p.Object, 3)
}

func (b *Bag) InitLink() {
	b.Object.Init(nil)
	b.Scores.Init(&b.Object, 1)
	b.Hold.Init(&b.Object, 2)
}

func (i *Item) InitLink() {
	i.Object.Init(nil)
	i.Id.Init(&i.Object, 1)
	i.Count.Init(&i.Object, 2)
}

func (c *Collections) InitLink(writer *diff.Writer) {
	c.Object.Init(writer)
	c.Scores.Init(&c.Object, 1)
	c.Numbers.Init(&c.Object, 2)
	c.Items.Init(&c.Object, 3)
	c.Order.Init(&c.Object, 4)
}

func (b *Bag) AppendDiffValue(data []byte) []byte {
	data = b.Scores.AppendValue(data, 1)
	return b.Hold.AppendValue(data, 2)
}

func (i *Item) AppendDiffValue(data []byte) []byte {
	data = i.Id.AppendValue(data, 1)
	return i.Count.AppendValue(data, 2)
}

func TestPrimitivePatchAndCommit(t *testing.T) {
	writer := diff.NewWriter()
	player := &Player{}
	player.InitLink(writer)

	player.Age.SetValue(18)
	player.Age.SetValue(20)

	patches := collectPatches(writer)
	if len(patches) != 1 {
		t.Fatalf("patch count = %d", len(patches))
	}
	assertPatch(t, patches[0], diff.Path{
		{KeyType: diff.PathField, FieldIndex: 2},
	}, diff.PrimitiveSet, int32(20))

	encoded, err := diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 1 || encoded[0].Operation != diff.PrimitiveSet {
		t.Fatalf("patches = %+v", encoded)
	}
	age, err := diff.DecodePrimitive[int32](encoded[0].Value)
	if err != nil || age != 20 {
		t.Fatalf("age = %d, err = %v", age, err)
	}
}

func TestPointerChildPath(t *testing.T) {
	writer := diff.NewWriter()
	player := &Player{}
	bag := &Bag{}
	item := &Item{}

	player.InitLink(writer)
	bag.InitLink()
	item.InitLink()

	bag.Hold.SetValue(item)
	player.Bag.SetValue(bag)
	writer.Reset()

	item.Count.SetValue(1001)

	patches := collectPatches(writer)
	if len(patches) != 1 {
		t.Fatalf("patch count = %d", len(patches))
	}
	assertPatch(t, patches[0], diff.Path{
		{KeyType: diff.PathField, FieldIndex: 1},
		{KeyType: diff.PathField, FieldIndex: 2},
		{KeyType: diff.PathField, FieldIndex: 2},
	}, diff.PrimitiveSet, int32(1001))

	player.Bag.SetValue(nil)
	patches = collectPatches(writer)
	if len(patches) != 1 || patches[0].Operation != diff.PointerClear {
		t.Fatalf("patches = %+v", patches)
	}
}

func TestMapOperations(t *testing.T) {
	writer := diff.NewWriter()
	collections := &Collections{}
	collections.InitLink(writer)

	collections.Scores.Store(7, 9)
	collections.Scores.Store(7, 10)
	collections.Scores.Store(8, 11)
	collections.Scores.Clear()
	collections.Scores.Store(7, 12)

	patches := collectPatches(writer)
	if len(patches) != 2 {
		t.Fatalf("patches = %+v", patches)
	}
	assertPatch(t, patches[0], diff.Path{
		{KeyType: diff.PathField, FieldIndex: 1},
	}, diff.MapClear, nil)
	assertPatch(t, patches[1], diff.Path{
		{KeyType: diff.PathMap, FieldIndex: 1, MapKey: int32(7)},
	}, diff.MapSet, int32(12))

	encoded, err := diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	key, err := diff.DecodePrimitive[int32](encoded[1].Path[0].MapKey)
	if err != nil || key != 7 {
		t.Fatalf("key = %d, err = %v", key, err)
	}
}

func TestSliceReplace(t *testing.T) {
	writer := diff.NewWriter()
	collections := &Collections{}
	collections.InitLink(writer)

	collections.Numbers.Append(10)
	writer.Reset()

	collections.Numbers.SetValue(0, 11)
	collections.Numbers.Insert(0, 20)
	collections.Numbers.SetValue(0, 21)
	collections.Numbers.SetValue(0, 22)

	patches := collectPatches(writer)
	if len(patches) != 1 {
		t.Fatalf("patches = %+v", patches)
	}
	assertPatch(t, patches[0], diff.Path{
		{KeyType: diff.PathField, FieldIndex: 2},
	}, diff.SliceReplace, &collections.Numbers)

	encoded, err := diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	values, err := diff.DecodeValues(encoded[0].Value)
	if err != nil || len(values) != 2 {
		t.Fatalf("values = %+v, err = %v", values, err)
	}
	first, err := diff.DecodePrimitive[int32](values[0])
	if err != nil || first != 22 {
		t.Fatalf("first = %d, err = %v", first, err)
	}
	second, err := diff.DecodePrimitive[int32](values[1])
	if err != nil || second != 11 {
		t.Fatalf("second = %d, err = %v", second, err)
	}

	writer.Reset()
	collections.Numbers.Clear()
	encoded, err = diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	values, err = diff.DecodeValues(encoded[0].Value)
	if err != nil || len(values) != 0 {
		t.Fatalf("values = %+v, err = %v", values, err)
	}
}

func TestPointerMultipleParents(t *testing.T) {
	writer := diff.NewWriter()
	collections := &Collections{}
	item := &Item{}
	collections.InitLink(writer)
	item.InitLink()

	collections.Items.Store(11, item)
	collections.Order.Append(item)
	writer.Reset()

	item.Count.SetValue(1001)

	patches := collectPatches(writer)
	if len(patches) != 2 {
		t.Fatalf("patch count = %d", len(patches))
	}
	assertPatch(t, patches[0], diff.Path{
		{KeyType: diff.PathMap, FieldIndex: 3, MapKey: int32(11)},
		{KeyType: diff.PathField, FieldIndex: 2},
	}, diff.PrimitiveSet, int32(1001))
	assertPatch(t, patches[1], diff.Path{
		{KeyType: diff.PathField, FieldIndex: 4},
	}, diff.SliceReplace, &collections.Order)
}

func TestPointerSliceUpdatesParentIndex(t *testing.T) {
	writer := diff.NewWriter()
	collections := &Collections{}
	first := &Item{}
	second := &Item{}
	collections.InitLink(writer)
	first.InitLink()
	second.InitLink()

	collections.Order.Append(first)
	collections.Order.Insert(0, second)
	writer.Reset()

	first.Count.SetValue(1)
	patches := collectPatches(writer)
	assertPatch(t, patches[0], diff.Path{
		{KeyType: diff.PathField, FieldIndex: 4},
	}, diff.SliceReplace, &collections.Order)

	collections.Order.Delete(1)
	writer.Reset()
	first.Count.SetValue(2)
	if writer.Len() != 0 {
		t.Fatalf("patches = %+v", collectPatches(writer))
	}
}

func TestPointerValueEncodedOnCommit(t *testing.T) {
	writer := diff.NewWriter()
	player := &Player{}
	bag := &Bag{}
	item := &Item{}
	player.InitLink(writer)
	bag.InitLink()
	item.InitLink()

	item.Id.SetValue(1001)
	item.Count.SetValue(5)
	bag.Hold.SetValue(item)
	player.Bag.SetValue(bag)

	encoded, err := diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 1 || encoded[0].Operation != diff.PointerSet {
		t.Fatalf("patches = %+v", encoded)
	}

	bagFields, err := diff.DecodeFields(encoded[0].Value)
	if err != nil || len(bagFields) != 1 || bagFields[0].FieldIndex != 2 {
		t.Fatalf("bag fields = %+v, err = %v", bagFields, err)
	}
	itemFields, err := diff.DecodeFields(bagFields[0].Value)
	if err != nil || len(itemFields) != 2 {
		t.Fatalf("item fields = %+v, err = %v", itemFields, err)
	}
	id, err := diff.DecodePrimitive[int32](itemFields[0].Value)
	if err != nil || id != 1001 {
		t.Fatalf("id = %d, err = %v", id, err)
	}
	count, err := diff.DecodePrimitive[int32](itemFields[1].Value)
	if err != nil || count != 5 {
		t.Fatalf("count = %d, err = %v", count, err)
	}
}

func TestPointerSlicePresence(t *testing.T) {
	writer := diff.NewWriter()
	collections := &Collections{}
	emptyItem := &Item{}
	collections.InitLink(writer)
	emptyItem.InitLink()

	collections.Order.Append(nil)
	collections.Order.Append(emptyItem)

	patches, err := diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 1 || patches[0].Operation != diff.SliceReplace {
		t.Fatalf("patches = %+v", patches)
	}
	patchValues, err := diff.DecodeValues(patches[0].Value)
	if err != nil || len(patchValues) != 2 {
		t.Fatalf("values = %+v, err = %v", patchValues, err)
	}
	if value, exists, err := diff.DecodePointerElement(patchValues[0]); err != nil || exists || value != nil {
		t.Fatalf("nil element value = %v, exists = %v, err = %v", value, exists, err)
	}
	if value, exists, err := diff.DecodePointerElement(patchValues[1]); err != nil || !exists || len(value) != 0 {
		t.Fatalf("empty element value = %v, exists = %v, err = %v", value, exists, err)
	}

	fullData := collections.Order.AppendValue(nil, 4)
	fields, err := diff.DecodeFields(fullData)
	if err != nil || len(fields) != 1 {
		t.Fatalf("fields = %+v, err = %v", fields, err)
	}
	values, err := diff.DecodeValues(fields[0].Value)
	if err != nil || len(values) != 2 {
		t.Fatalf("values = %+v, err = %v", values, err)
	}
	if _, exists, err := diff.DecodePointerElement(values[0]); err != nil || exists {
		t.Fatalf("first exists = %v, err = %v", exists, err)
	}
	if value, exists, err := diff.DecodePointerElement(values[1]); err != nil || !exists || len(value) != 0 {
		t.Fatalf("second value = %v, exists = %v, err = %v", value, exists, err)
	}
}

func TestCollectionFullValue(t *testing.T) {
	writer := diff.NewWriter()
	collections := &Collections{}
	collections.InitLink(writer)
	collections.Scores.Store(7, 9)
	collections.Numbers.Append(10)
	collections.Numbers.Append(20)

	mapData := collections.Scores.AppendValue(nil, 1)
	mapFields, err := diff.DecodeFields(mapData)
	if err != nil || len(mapFields) != 1 {
		t.Fatalf("map fields = %+v, err = %v", mapFields, err)
	}
	mapValues, err := diff.DecodeValues(mapFields[0].Value)
	if err != nil || len(mapValues) != 2 {
		t.Fatalf("map values = %+v, err = %v", mapValues, err)
	}
	key, err := diff.DecodePrimitive[int32](mapValues[0])
	if err != nil || key != 7 {
		t.Fatalf("key = %d, err = %v", key, err)
	}
	value, err := diff.DecodePrimitive[int32](mapValues[1])
	if err != nil || value != 9 {
		t.Fatalf("value = %d, err = %v", value, err)
	}

	sliceData := collections.Numbers.AppendValue(nil, 2)
	sliceFields, err := diff.DecodeFields(sliceData)
	if err != nil || len(sliceFields) != 1 {
		t.Fatalf("slice fields = %+v, err = %v", sliceFields, err)
	}
	sliceValues, err := diff.DecodeValues(sliceFields[0].Value)
	if err != nil || len(sliceValues) != 2 {
		t.Fatalf("slice values = %+v, err = %v", sliceValues, err)
	}
	first, err := diff.DecodePrimitive[int32](sliceValues[0])
	if err != nil || first != 10 {
		t.Fatalf("first = %d, err = %v", first, err)
	}
	second, err := diff.DecodePrimitive[int32](sliceValues[1])
	if err != nil || second != 20 {
		t.Fatalf("second = %d, err = %v", second, err)
	}
}

func TestDecodeRejectsInvalidData(t *testing.T) {
	if _, err := diff.DecodePatches([]byte{1}); err == nil {
		t.Fatal("expected invalid patch error")
	}
	if _, err := diff.DecodeFields([]byte{1, 2, 1}); err == nil {
		t.Fatal("expected invalid field error")
	}
	if _, _, err := diff.DecodePointerElement(nil); err == nil {
		t.Fatal("expected invalid pointer element error")
	}
	if _, err := diff.DecodePrimitive[int32](nil); err == nil {
		t.Fatal("expected invalid primitive error")
	}
}

func TestDecodePrimitive(t *testing.T) {
	testPrimitive(t, true)
	testPrimitive(t, Status(3))
	testPrimitive(t, int64(-1001))
	testPrimitive(t, uint64(1001))
	testPrimitive(t, float32(1.5))
	testPrimitive(t, float64(2.5))
	testPrimitive(t, complex64(complex(1, 2)))
	testPrimitive(t, complex(3.0, 4.0))
	testPrimitive(t, "nova")
}

type Status int32

func testPrimitive[T interface {
	~bool | ~int32 | ~int64 | ~uint64 | ~float32 | ~float64 | ~complex64 | ~complex128 | ~string
}](t *testing.T, value T) {
	t.Helper()
	writer := diff.NewWriter()
	object := &primitiveObject[T]{}
	object.Object.Init(writer)
	object.Value.Init(&object.Object, 1)
	object.Value.SetValue(value)

	patches, err := diff.DecodePatches(writer.Commit())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := diff.DecodePrimitive[T](patches[0].Value)
	if err != nil || decoded != value {
		t.Fatalf("decoded = %v, want %v, err = %v", decoded, value, err)
	}
}

type primitiveObject[T interface {
	~bool | ~int32 | ~int64 | ~uint64 | ~float32 | ~float64 | ~complex64 | ~complex128 | ~string
}] struct {
	diff.Object
	Value diff.Primitive[T]
}

func collectPatches(writer *diff.Writer) []diff.Patch {
	patches := make([]diff.Patch, 0, writer.Len())
	writer.Range(func(patch diff.Patch) bool {
		patches = append(patches, patch)
		return true
	})
	return patches
}

func assertPatch(t *testing.T, patch diff.Patch, path diff.Path, operation diff.Operation, value any) {
	t.Helper()
	if !reflect.DeepEqual(patch.Path, path) {
		t.Fatalf("path = %+v, want %+v", patch.Path, path)
	}
	if patch.Operation != operation {
		t.Fatalf("operation = %d, want %d", patch.Operation, operation)
	}
	if patch.Value != value {
		t.Fatalf("value = %v, want %v", patch.Value, value)
	}
}
