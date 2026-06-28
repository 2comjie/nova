package gob

import (
	"bytes"
	"encoding/gob"
)

type GobCodec struct {
}

func (g GobCodec) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g GobCodec) Unmarshal(data []byte, v any) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(v)
}

func (g GobCodec) Name() string {
	return "gob"
}
