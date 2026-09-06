package main

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

//go:embed testdata/fixture_test.go.txt
var fixtureTest []byte

func TestGeneratedService(t *testing.T) {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("fixture.proto"),
		Package: proto.String("fixture"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/2comjie/nova/rpctest;fixture")},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Request")},
			{Name: proto.String("Response")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("Sample"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name: proto.String("Unary"), InputType: proto.String(".fixture.Request"), OutputType: proto.String(".fixture.Response"),
			}},
		}},
	}
	gen, err := (protogen.Options{}).New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"fixture.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{file},
		Parameter:      proto.String("paths=source_relative"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := generateFile(gen, gen.Files[0]); err != nil {
		t.Fatal(err)
	}
	response := gen.Response()
	if response.GetError() != "" {
		t.Fatal(response.GetError())
	}
	content := response.File[0].GetContent()
	for _, name := range []string{"grpc", "Stream", "ServiceDesc", "interceptor"} {
		if strings.Contains(content, name) {
			t.Errorf("generated unnecessary code: %s", name)
		}
	}
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "fixture_rpc.pb.go")
	if err := os.WriteFile(generatedFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(dir, "fixture_test.go")
	if err := os.WriteFile(testFile, fixtureTest, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), filepath.Join(runtime.GOROOT(), "bin", "go"), "test", generatedFile, testFile)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated service tests failed: %v\n%s", err, output)
	}
}

func TestRejectStreaming(t *testing.T) {
	for _, mode := range []string{"client", "server", "bidirectional"} {
		t.Run(mode, func(t *testing.T) {
			file := &descriptorpb.FileDescriptorProto{
				Name: proto.String("stream.proto"), Package: proto.String("fixture"), Syntax: proto.String("proto3"),
				Options:     &descriptorpb.FileOptions{GoPackage: proto.String("github.com/2comjie/nova/rpctest;fixture")},
				MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Message")}},
				Service: []*descriptorpb.ServiceDescriptorProto{{
					Name: proto.String("Sample"),
					Method: []*descriptorpb.MethodDescriptorProto{{
						Name: proto.String("Stream"), InputType: proto.String(".fixture.Message"), OutputType: proto.String(".fixture.Message"),
						ClientStreaming: proto.Bool(mode != "server"), ServerStreaming: proto.Bool(mode != "client"),
					}},
				}},
			}
			gen, err := (protogen.Options{}).New(&pluginpb.CodeGeneratorRequest{
				FileToGenerate: []string{"stream.proto"}, ProtoFile: []*descriptorpb.FileDescriptorProto{file},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := generateFile(gen, gen.Files[0]); err == nil || !strings.Contains(err.Error(), "streaming is not supported") {
				t.Fatalf("streaming error=%v", err)
			}
			if len(gen.Response().File) != 0 {
				t.Fatal("streaming service generated a partial file")
			}
		})
	}
}
