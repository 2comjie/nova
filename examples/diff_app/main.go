package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/examples/diff_app/profile"
	pbProfile "github.com/2comjie/nova/examples/pb/profile"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	value := new(profile.Profile)
	value.SetId(1001)
	value.SetState(profile.Online)
	value.SetLoginAt(time.UnixMilli(1700000000123))
	value.SetCooldown(1500*time.Millisecond + 5*time.Nanosecond)
	value.History().Append(profile.Online)
	value.RewardsAt().Store("daily", value.GetLoginAt())

	value.InitLink(diff.NewWriter())
	body, err := proto.Marshal(value.Snapshot())
	if err != nil {
		return err
	}
	snapshot := new(pbProfile.Profile)
	if err := proto.Unmarshal(body, snapshot); err != nil {
		return err
	}
	replica := new(profile.Profile)
	replica.LoadSnapshot(snapshot)
	fmt.Printf("全量 Proto: %d bytes，时间戳: %d ms，时长: %d ns\n", len(body), snapshot.LoginAt, snapshot.Cooldown)

	value.SetState(profile.Offline)
	value.History().Append(profile.Offline)
	value.RewardsAt().Store("daily", value.GetLoginAt().Add(time.Hour))
	body, err = proto.Marshal(value.Commit())
	if err != nil {
		return err
	}
	update := new(pbProfile.Profile)
	if err := proto.Unmarshal(body, update); err != nil {
		return err
	}
	replica.Merge(update)
	if !proto.Equal(value.Snapshot(), replica.Snapshot()) {
		panic("增量合并结果不一致")
	}
	fmt.Printf("增量 Proto: %d bytes，合并后状态: %d\n", len(body), replica.GetState())

	for _, codec := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{"JSON", json.Marshal, json.Unmarshal},
		{"BSON", bson.Marshal, bson.Unmarshal},
	} {
		body, err := codec.marshal(value)
		if err != nil {
			return err
		}
		restored := new(profile.Profile)
		if err := codec.unmarshal(body, restored); err != nil {
			return err
		}
		if !proto.Equal(value.Snapshot(), restored.Snapshot()) {
			panic(codec.name + " 往返结果不一致")
		}
		fmt.Printf("%s 往返通过: %d bytes\n", codec.name, len(body))
		if codec.name == "JSON" {
			fmt.Println(string(body))
		}
	}
	return nil
}
