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
			1001: {
				Id: 1001, Count: 10, Quality: testdata.ItemQuality_ITEM_QUALITY_COMMON, Name: "sword", Payload: []byte{1},
				Attr:    &testdata.ItemAttr{Level: 1, Durability: 100},
				Effects: map[int32]*testdata.ItemEffect{1: {Id: 1, Power: 10}},
				Gems:    []*testdata.ItemGem{{Id: 1, Level: 1}},
			},
			1002: {Id: 1002, Count: 20, Quality: testdata.ItemQuality_ITEM_QUALITY_RARE, Name: "shield", Payload: []byte{2}},
		},
		Currencies:   map[string]int64{"gold": 100, "silver": 200},
		Snapshots:    map[int32][]byte{1: {1, 1}},
		FeatureFlags: map[bool]string{true: "enabled", false: "disabled"},
		Checkpoints:  []int64{10, 20, 30},
		Labels:       []string{"a", "b"},
		Packets:      [][]byte{{1}, {2}},
		Lineup: []*testdata.InventoryItem{
			{Id: 2001, Count: 1, Name: "first", Attr: &testdata.ItemAttr{Level: 2, Durability: 80}},
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
	item.GetAttr().SetDurability(90)
	effect, ok := item.Effects().GetValue(1)
	if !ok {
		t.Fatal("item effect 1 not found")
	}
	effect.SetPower(20)
	gem, ok := item.Gems().GetValue(0)
	if !ok {
		t.Fatal("item gem 0 not found")
	}
	gem.SetLevel(2)
	item.Gems().Append(&testdata.ItemGem{Id: 2, Level: 1})
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
	secondLineupValue, ok := lineup.GetValue(1)
	if !ok {
		t.Fatal("lineup 1 not found")
	}
	lineupValue.SetCount(10)
	lineupValue.GetAttr().SetDurability(70)
	lineup.Move(0, 1)
	secondLineupValue.SetCount(20)
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

func TestGeneratedMessageStateHandles(t *testing.T) {
	source := &testdata.GameData{
		Items: map[uint64]*testdata.InventoryItem{
			1: {Id: 1, Count: 1, Attr: &testdata.ItemAttr{Level: 1}},
		},
		Lineup: []*testdata.InventoryItem{
			{Id: 10, Count: 10},
			{Id: 20, Count: 20},
		},
	}
	target := proto.Clone(source).(*testdata.GameData)
	state := testdata.NewGameDataState(source)

	items := state.Items()
	item, _ := items.GetValue(1)
	sameItem, _ := items.GetValue(1)
	if item != sameItem {
		t.Fatal("map must return the same cached message state")
	}
	item.GetAttr().SetLevel(2)

	lineup := state.Lineup()
	moved, _ := lineup.GetValue(1)
	lineup.Move(1, 0)
	moved.SetCount(21)

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := testdata.ApplyGameDataDiff(target, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(target, source) {
		t.Fatalf("diff result does not match source\nsource: %v\nresult: %v", source, target)
	}

	state.ClearDirty()
	items.Delete(1)
	lineup.Delete(0)
	state.ClearDirty()
	item.SetCount(99)
	moved.SetCount(99)
	if state.IsDirty() {
		t.Fatal("removed message states must not dirty their former parent")
	}
}

func TestGeneratedApplyHooks(t *testing.T) {
	source := &testdata.GameData{
		Profile: &testdata.Profile{Nickname: "before"},
		Items: map[uint64]*testdata.InventoryItem{
			1001: {
				Id:      1001,
				Count:   10,
				Attr:    &testdata.ItemAttr{Level: 1, Durability: 100},
				Effects: map[int32]*testdata.ItemEffect{1: {Id: 1, Power: 10}},
			},
		},
		Currencies:  map[string]int64{"silver": 20},
		Snapshots:   map[int32][]byte{1: {1}},
		Checkpoints: []int64{10},
	}
	target := proto.Clone(source).(*testdata.GameData)
	state := testdata.NewGameDataState(source)
	state.GetProfile().SetNickname("after")
	item, _ := state.Items().GetValue(1001)
	item.SetCount(7)
	item.GetAttr().SetDurability(90)
	effect, _ := item.Effects().GetValue(1)
	effect.SetPower(20)
	state.Currencies().Store("gold", 100)
	state.Currencies().Delete("silver")
	state.Snapshots().Clear()
	state.Checkpoints().Append(20)

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	toastCount := 0
	putCount := 0
	deleteCount := 0
	clearCount := 0
	appendCount := 0
	nicknameCount := 0
	attrCount := 0
	effectCount := 0
	hooks := &testdata.GameDataApplyHooks{
		Items: func(itemKey uint64) *testdata.InventoryItemApplyHooks {
			if itemKey != 1001 {
				t.Fatalf("unexpected item hook key: %d", itemKey)
			}
			return &testdata.InventoryItemApplyHooks{
				Attr: &testdata.ItemAttrApplyHooks{
					OnDurabilityChanged: func(oldValue, newValue int32) {
						if oldValue != 100 || newValue != 90 {
							t.Fatalf("unexpected durability event: old=%d new=%d", oldValue, newValue)
						}
						attrCount++
					},
				},
				Effects: func(effectKey int32) *testdata.ItemEffectApplyHooks {
					if effectKey != 1 {
						t.Fatalf("unexpected effect hook key: %d", effectKey)
					}
					return &testdata.ItemEffectApplyHooks{
						OnPowerChanged: func(oldValue, newValue int32) {
							if oldValue != 10 || newValue != 20 {
								t.Fatalf("unexpected effect event: old=%d new=%d", oldValue, newValue)
							}
							effectCount++
						},
					}
				},
			}
		},
		OnItemsCountChanged: func(key uint64, oldValue, newValue int32) {
			if key != 1001 || oldValue != 10 || newValue != 7 {
				t.Fatalf("unexpected item count event: key=%d old=%d new=%d", key, oldValue, newValue)
			}
			toastCount = int(oldValue - newValue)
		},
		OnCurrenciesPut: func(key string, oldValue, newValue int64, replaced bool) {
			if key != "gold" || oldValue != 0 || newValue != 100 || replaced {
				t.Fatalf("unexpected map put event: key=%s old=%d new=%d replaced=%v", key, oldValue, newValue, replaced)
			}
			putCount++
		},
		OnCurrenciesDelete: func(key string, oldValue int64) {
			if key != "silver" || oldValue != 20 {
				t.Fatalf("unexpected map delete event: key=%s old=%d", key, oldValue)
			}
			deleteCount++
		},
		OnSnapshotsClear: func() {
			clearCount++
		},
		OnCheckpointsAppend: func(index int, value int64) {
			if index != 1 || value != 20 {
				t.Fatalf("unexpected list append event: index=%d value=%d", index, value)
			}
			appendCount++
		},
		OnProfilePatch: func(oldValue, newValue *testdata.Profile) {
			if oldValue.Nickname != "before" || newValue.Nickname != "after" {
				t.Fatalf("unexpected profile event: old=%s new=%s", oldValue.Nickname, newValue.Nickname)
			}
			nicknameCount++
		},
	}
	if err := testdata.ApplyGameDataDiffWithHooks(target, writer.Data(), hooks); err != nil {
		t.Fatal(err)
	}
	if toastCount != 3 || putCount != 1 || deleteCount != 1 || clearCount != 1 || appendCount != 1 || nicknameCount != 1 || attrCount != 1 || effectCount != 1 {
		t.Fatalf("unexpected hook counts: toast=%d put=%d delete=%d clear=%d append=%d nickname=%d attr=%d effect=%d", toastCount, putCount, deleteCount, clearCount, appendCount, nicknameCount, attrCount, effectCount)
	}
	if !proto.Equal(target, source) {
		t.Fatalf("diff result does not match source\nsource: %v\nresult: %v", source, target)
	}
}
