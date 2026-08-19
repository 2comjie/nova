package diff_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/2comjie/nova/core/util"
	"github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

func TestProtoStateSyncBetweenServerAndClient(t *testing.T) {
	serverValue := &testdata.GameData{
		Scalars: &testdata.ScalarValues{
			Enabled: true,
			Quality: testdata.ItemQuality_ITEM_QUALITY_COMMON,
		},
		Items: map[uint64]*testdata.InventoryItem{
			1: {
				Id:      1,
				Count:   100,
				Quality: testdata.ItemQuality_ITEM_QUALITY_COMMON,
				Name:    "sword",
			},
		},
		Currencies:  map[string]int64{"coin": 1000},
		Checkpoints: []int64{1},
		Qualities:   []testdata.ItemQuality{testdata.ItemQuality_ITEM_QUALITY_COMMON},
	}
	clientValue := &testdata.GameData{}
	serverState := testdata.NewGameDataState(serverValue)
	manager := diff.NewSnapManager[*testdata.GameData](serverState, 100, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pushes := make(chan []byte)
	errs := make(chan error, 1)
	var wait sync.WaitGroup
	wait.Add(2)

	go func() {
		defer wait.Done()
		defer close(pushes)

		manager.BuildFull()
		fullVersion, fullData, deltas := manager.Get(0)
		writer := diff.NewSyncWriter(nil)
		writer.WriteFull(fullVersion, fullData, deltas)
		select {
		case pushes <- writer.Data():
		case <-ctx.Done():
			return
		}

		for index := 1; index <= 200; index++ {
			baseVersion := manager.Version()
			quality := testdata.ItemQuality_ITEM_QUALITY_COMMON
			switch index % 3 {
			case 1:
				quality = testdata.ItemQuality_ITEM_QUALITY_RARE
			case 2:
				quality = testdata.ItemQuality_ITEM_QUALITY_EPIC
			}

			serverState.GetScalars().SetQuality(quality)
			item, _ := serverState.Items().GetValue(1)
			item.SetCount(int32(100 + index))
			item.SetQuality(quality)
			serverState.Currencies().Store("coin", int64(1000+index*10))
			serverState.Qualities().Store(0, quality)
			serverState.Snapshots().Store(int32(index%5), []byte{byte(index), byte(index >> 8)})
			if index%2 == 0 {
				serverState.Items().Store(2, &testdata.InventoryItem{Id: 2, Count: int32(index), Quality: quality, Name: "shield"})
			} else {
				serverState.Items().Delete(2)
			}
			manager.Commit()

			_, fullData, deltas = manager.Get(baseVersion)
			if fullData != nil || len(deltas) != 1 {
				select {
				case errs <- fmt.Errorf("服务端增量错误: full=%d deltas=%d", len(fullData), len(deltas)):
				default:
				}
				cancel()
				return
			}
			writer = diff.NewSyncWriter(nil)
			writer.WriteDiff(deltas)
			select {
			case pushes <- writer.Data():
			case <-ctx.Done():
				return
			}
		}
	}()

	var clientVersion uint64
	go func() {
		defer wait.Done()

		for body := range pushes {
			reader, err := diff.NewSyncReader(body)
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				cancel()
				return
			}

			if reader.HasFull() {
				if err := proto.Unmarshal(reader.FullData(), clientValue); err != nil {
					select {
					case errs <- err:
					default:
					}
					cancel()
					return
				}
				clientVersion = reader.BaseVersion()
			} else if clientVersion != reader.BaseVersion() {
				select {
				case errs <- fmt.Errorf("客户端版本不连续: local=%d base=%d", clientVersion, reader.BaseVersion()):
				default:
				}
				cancel()
				return
			}

			for {
				delta, ok, err := reader.NextDiff()
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					cancel()
					return
				}
				if !ok {
					break
				}
				if err := testdata.ApplyGameDataDiff(clientValue, delta.Data); err != nil {
					select {
					case errs <- err:
					default:
					}
					cancel()
					return
				}
				clientVersion = delta.Version
			}
			if clientVersion != reader.Version() {
				select {
				case errs <- fmt.Errorf("客户端最终版本错误: local=%d target=%d", clientVersion, reader.Version()):
				default:
				}
				cancel()
				return
			}
		}
	}()

	wait.Wait()
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
	if clientVersion != manager.Version() {
		t.Fatalf("最终版本不一致: client=%d server=%d", clientVersion, manager.Version())
	}
	if !proto.Equal(clientValue, serverValue) {
		t.Fatalf("最终Proto不一致:\nclient=%v\nserver=%v", clientValue, serverValue)
	}

	util.WaitUntilSignaled()
}
