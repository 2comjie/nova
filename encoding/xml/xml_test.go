package xml

import "testing"

func TestXmlCodecUnmarshalMap(t *testing.T) {
	var got map[string]any
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?><root><name>nova</name><port>8080</port></root>`)

	if err := (&XmlCodec{}).Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	root, ok := got["root"].(map[string]any)
	if !ok {
		t.Fatalf("got root = %#v, want map[string]any", got["root"])
	}
	if root["name"] != "nova" {
		t.Fatalf("got root.name = %#v, want %q", root["name"], "nova")
	}
	if root["port"] != "8080" {
		t.Fatalf("got root.port = %#v, want %q", root["port"], "8080")
	}
}

func TestXmlCodecUnmarshalMapWithGBKEncoding(t *testing.T) {
	var got map[string]any
	data := append([]byte(`<?xml version="1.0" encoding="GBK"?><root><name>`),
		0xd6, 0xd0, 0xce, 0xc4, // "中文" in GBK
	)
	data = append(data, []byte(`</name></root>`)...)

	if err := (&XmlCodec{}).Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	root, ok := got["root"].(map[string]any)
	if !ok {
		t.Fatalf("got root = %#v, want map[string]any", got["root"])
	}
	if root["name"] != "中文" {
		t.Fatalf("got root.name = %#v, want %q", root["name"], "中文")
	}
}
