package diff_test

import (
	"testing"

	. "github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

func TestGeneratedComplexStateEndToEnd(t *testing.T) {
	original := &testdata.GameData{
		Scalars: &testdata.ScalarValues{
			Enabled:       true,
			Quality:       testdata.ItemQuality_ITEM_QUALITY_COMMON,
			Int32Value:    1,
			Sint32Value:   -2,
			Sfixed32Value: -3,
			Uint32Value:   4,
			Fixed32Value:  5,
			Int64Value:    6,
			Sint64Value:   -7,
			Sfixed64Value: -8,
			Uint64Value:   9,
			Fixed64Value:  10,
			FloatValue:    1.5,
			DoubleValue:   2.5,
			StringValue:   "before",
			BytesValue:    []byte{1, 2},
		},
		Profile: &testdata.Profile{
			Nickname: "player",
			Address:  &testdata.Address{Country: "CN", City: "Shanghai", ZipCode: 200000},
			Tags:     []string{"old", "vip"},
		},
		Items: map[uint64]*testdata.InventoryItem{
			1001: {Id: 1001, Count: 10, Quality: testdata.ItemQuality_ITEM_QUALITY_COMMON, Name: "sword", Payload: []byte{1}},
			1002: {Id: 1002, Count: 20, Quality: testdata.ItemQuality_ITEM_QUALITY_RARE, Name: "shield", Payload: []byte{2}},
		},
		Currencies:   map[string]int64{"gold": 100, "silver": 200},
		Snapshots:    map[int32][]byte{1: {1, 1}},
		FeatureFlags: map[bool]string{true: "enabled", false: "disabled"},
		Checkpoints:  []int64{10, 20, 30},
		Labels:       []string{"a", "b"},
		Packets:      [][]byte{{1}, {2}},
		Lineup: []*testdata.InventoryItem{
			{Id: 2001, Count: 1, Name: "first"},
			{Id: 2002, Count: 2, Name: "second"},
		},
		Qualities: []testdata.ItemQuality{
			testdata.ItemQuality_ITEM_QUALITY_COMMON,
			testdata.ItemQuality_ITEM_QUALITY_RARE,
		},
		Wide: &testdata.WideState{V1: 1, V64: 64, V65: 65, V70: 70},
	}
	replica := proto.Clone(original).(*testdata.GameData)
	state := testdata.NewGameDataState(original)

	scalars := state.GetScalars()
	scalars.SetEnabled(false)
	scalars.SetQuality(testdata.ItemQuality_ITEM_QUALITY_EPIC)
	scalars.SetInt32Value(-101)
	scalars.SetSint32Value(-102)
	scalars.SetSfixed32Value(-103)
	scalars.SetUint32Value(104)
	scalars.SetFixed32Value(105)
	scalars.SetInt64Value(-106)
	scalars.SetSint64Value(-107)
	scalars.SetSfixed64Value(-108)
	scalars.SetUint64Value(109)
	scalars.SetFixed64Value(110)
	scalars.SetFloatValue(11.5)
	scalars.SetDoubleValue(12.5)
	scalars.SetStringValue("after")
	bytesValue := []byte{9, 8, 7}
	scalars.SetBytesValue(bytesValue)
	bytesValue[0] = 0

	profile := state.GetProfile()
	profile.SetNickname("renamed")
	profile.GetAddress().SetCity("Hangzhou")
	profile.GetAddress().SetZipCode(310000)
	profile.Tags().Store(0, "new")
	profile.Tags().Append("active")

	wide := state.GetWide()
	wide.SetV1(101)
	wide.SetV64(164)
	wide.SetV65(165)
	wide.SetV70(170)

	items := state.Items()
	item, ok := items.GetValue(1001)
	if !ok {
		t.Fatal("item 1001 not found")
	}
	item.SetCount(11)
	item.SetQuality(testdata.ItemQuality_ITEM_QUALITY_EPIC)
	item.SetPayload([]byte{3, 4})
	items.Delete(1002)
	items.Store(1003, &testdata.InventoryItem{Id: 1003, Count: 30, Name: "helmet"})

	state.Currencies().Store("gold", 999)
	state.Currencies().Delete("silver")
	snapshot := []byte{5, 6}
	state.Snapshots().Store(2, snapshot)
	snapshot[0] = 0
	state.FeatureFlags().Clear()
	state.FeatureFlags().Store(true, "new")

	checkpoints := state.Checkpoints()
	checkpoints.Store(0, 11)
	checkpoints.Append(40)
	checkpoints.Insert(1, 15)
	checkpoints.Delete(3)
	checkpoints.Move(2, 0)

	state.Labels().Clear()
	state.Labels().Append("reset")
	packet := []byte{7, 8}
	state.Packets().Append(packet)
	packet[0] = 0

	lineup := state.Lineup()
	lineupValue, ok := lineup.GetValue(0)
	if !ok {
		t.Fatal("lineup 0 not found")
	}
	lineupValue.SetCount(10)
	lineup.Move(0, 1)
	replacement := &testdata.InventoryItem{Id: 3001, Count: 3, Name: "replacement"}
	lineup.Store(0, replacement)
	replacement.Count = 999

	qualities := state.Qualities()
	qualities.Store(0, testdata.ItemQuality_ITEM_QUALITY_EPIC)
	qualities.Append(testdata.ItemQuality_ITEM_QUALITY_UNKNOWN)
	qualities.Move(2, 1)

	if !state.IsDirty() {
		t.Fatal("complex state must be dirty")
	}
	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := testdata.ApplyGameDataDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("diff result does not match source\nsource: %v\nresult: %v", original, replica)
	}
	state.ClearDirty()
	if state.IsDirty() {
		t.Fatal("ClearDirty did not clear root state")
	}
}
