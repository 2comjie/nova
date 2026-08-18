package diffgen

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestGenerateStateAndDirtyBits(t *testing.T) {
	request := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"player.proto"},
		ProtoFile: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("player.proto"),
			Package: proto.String("game.player"),
			Syntax:  proto.String("proto3"),
			Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/acme/game/player;player")},
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("Player"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("level"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()},
					{Name: proto.String("name"), Number: proto.Int32(2), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
					{Name: proto.String("bag"), Number: proto.Int32(3), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".game.player.Bag")},
				},
			}, {
				Name: proto.String("Bag"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("capacity"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()},
				},
			}},
		}},
	}
	plugin, err := (protogen.Options{}).New(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := Generate(plugin); err != nil {
		t.Fatal(err)
	}
	response := plugin.Response()
	if len(response.File) != 1 {
		t.Fatalf("expected one generated file, got %d", len(response.File))
	}
	content := response.File[0].GetContent()
	for _, expected := range []string{
		"type PlayerState struct",
		"func NewPlayerState(value *Player) *PlayerState",
		"const playerStateDirtyLevelWord uint32 = 0",
		"const playerStateDirtyLevelMask uint64 = 1",
		"const playerStateDirtyNameMask uint64 = 2",
		"func (s *PlayerState) LoadLevel() int32",
		"func (s *PlayerState) StoreLevel(value int32)",
		"func (s *PlayerState) LoadName() string",
		"func (s *PlayerState) StoreName(value string)",
		"func (s *PlayerState) WriteDiff(writer *diff.Writer)",
		"func ApplyPlayerDiff(value *Player, data []byte) error",
		"func (s *PlayerState) LoadBag() (*BagState, bool)",
		"func (s *PlayerState) StoreBag(value *Bag)",
		"func (s *PlayerState) DeleteBag()",
		"func ApplyBagDiff(value *Bag, data []byte) error",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("generated file does not contain %q\n%s", expected, content)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), response.File[0].GetName(), content, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedBagFileIsCurrent(t *testing.T) {
	descriptor := protodesc.ToFileDescriptorProto(testdata.File_diff_testdata_bag_proto)
	request := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{descriptor.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{descriptor},
		Parameter:      proto.String("paths=source_relative"),
	}
	plugin, err := (protogen.Options{}).New(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := Generate(plugin); err != nil {
		t.Fatal(err)
	}
	response := plugin.Response()
	if len(response.File) != 1 {
		t.Fatalf("expected one generated file, got %d", len(response.File))
	}
	actual, err := os.ReadFile("../../diff/testdata/bag_diff.pb.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != response.File[0].GetContent() {
		t.Fatal("bag_diff.pb.go is stale; regenerate it with protoc-gen-go-diff")
	}
}
