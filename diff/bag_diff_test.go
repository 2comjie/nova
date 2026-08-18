package diff

import (
	"testing"

	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

func TestBagDiff(t *testing.T) {
	original := &testdata.Player{Bag: &testdata.Bag{
		Capacity: 50,
		Items: map[uint64]*testdata.Item{
			1001: {Id: 1001, Count: 10, Level: 2},
			3001: {Id: 3001, Count: 30, Level: 4},
		},
		Order: []uint64{1001, 3001},
	}}

	expected := proto.Clone(original).(*testdata.Player)
	expected.Bag.Capacity = 100
	expected.Bag.Items[1001].Count = 20
	expected.Bag.Items[2001] = &testdata.Item{Id: 2001, Count: 1, Level: 5}
	delete(expected.Bag.Items, 3001)
	expected.Bag.Order = append(expected.Bag.Order, 2001)

	newItem, err := proto.Marshal(expected.Bag.Items[2001])
	if err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(nil)
	writer.Patch(1, func(writer *Writer) {
		writer.Int32(1, expected.Bag.Capacity)
		writer.MapPatch(2, func(writer *Writer) {
			writer.AppendUint64(1001)
		}, func(writer *Writer) {
			writer.Int32(2, expected.Bag.Items[1001].Count)
		})
		writer.MapPut(2, func(writer *Writer) {
			writer.AppendUint64(2001)
		}, func(writer *Writer) {
			writer.AppendBytes(newItem)
		})
		writer.MapDelete(2, func(writer *Writer) {
			writer.AppendUint64(3001)
		})
		writer.ListAppend(3, func(writer *Writer) {
			writer.AppendUint64(2001)
		})
	})

	actual := proto.Clone(original).(*testdata.Player)
	if err := applyPlayerDiff(actual, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(actual, expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func applyPlayerDiff(player *testdata.Player, data []byte) error {
	reader := NewReader(data)
	for {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil || !ok {
			return err
		}

		if fieldNumber != 1 {
			continue
		}
		switch operation {
		case OperationPatch:
			if player.Bag == nil {
				player.Bag = &testdata.Bag{}
			}
			if err := applyBagDiff(player.Bag, payload); err != nil {
				return err
			}
		case OperationReplace:
			player.Bag = &testdata.Bag{}
			if err := proto.Unmarshal(payload, player.Bag); err != nil {
				return err
			}
		case OperationClear:
			player.Bag = nil
		default:
			return ErrInvalidData
		}
	}
}

func applyBagDiff(bag *testdata.Bag, data []byte) error {
	reader := NewReader(data)
	for {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil || !ok {
			return err
		}

		switch fieldNumber {
		case 1:
			if operation != OperationSetVarint {
				return ErrInvalidData
			}
			bag.Capacity = DecodeInt32(payload)
		case 2:
			valueReader := NewValueReader(payload)
			key := valueReader.Uint64()
			switch operation {
			case OperationMapPut:
				itemData := valueReader.Bytes()
				if valueReader.Err() != nil || !valueReader.Done() {
					return ErrInvalidData
				}
				item := &testdata.Item{}
				if err := proto.Unmarshal(itemData, item); err != nil {
					return err
				}
				if bag.Items == nil {
					bag.Items = make(map[uint64]*testdata.Item)
				}
				bag.Items[key] = item
			case OperationMapDelete:
				if valueReader.Err() != nil || !valueReader.Done() {
					return ErrInvalidData
				}
				delete(bag.Items, key)
			case OperationMapPatch:
				patch := valueReader.Remaining()
				if valueReader.Err() != nil {
					return ErrInvalidData
				}
				item := bag.Items[key]
				if item == nil {
					item = &testdata.Item{}
					if bag.Items == nil {
						bag.Items = make(map[uint64]*testdata.Item)
					}
					bag.Items[key] = item
				}
				if err := applyItemDiff(item, patch); err != nil {
					return err
				}
			default:
				return ErrInvalidData
			}
		case 3:
			valueReader := NewValueReader(payload)
			switch operation {
			case OperationListAppend:
				bag.Order = append(bag.Order, valueReader.Uint64())
			case OperationListInsert:
				index := int(valueReader.Uint32())
				value := valueReader.Uint64()
				bag.Order = append(bag.Order, 0)
				copy(bag.Order[index+1:], bag.Order[index:len(bag.Order)-1])
				bag.Order[index] = value
			case OperationListSet:
				index := int(valueReader.Uint32())
				bag.Order[index] = valueReader.Uint64()
			case OperationListDelete:
				index := int(valueReader.Uint32())
				copy(bag.Order[index:], bag.Order[index+1:])
				bag.Order = bag.Order[:len(bag.Order)-1]
			case OperationListMove:
				from := int(valueReader.Uint32())
				to := int(valueReader.Uint32())
				value := bag.Order[from]
				if from < to {
					copy(bag.Order[from:to], bag.Order[from+1:to+1])
				} else {
					copy(bag.Order[to+1:from+1], bag.Order[to:from])
				}
				bag.Order[to] = value
			default:
				return ErrInvalidData
			}
			if valueReader.Err() != nil || !valueReader.Done() {
				return ErrInvalidData
			}
		}
	}
}

func applyItemDiff(item *testdata.Item, data []byte) error {
	reader := NewReader(data)
	for {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil || !ok {
			return err
		}

		if fieldNumber != 2 {
			continue
		}
		if operation != OperationSetVarint {
			return ErrInvalidData
		}
		item.Count = DecodeInt32(payload)
	}
}
