package zipper

import "github.com/klauspost/compress/snappy"

type Snappy struct {
	maxSize int
}

func NewSnappy(maxSize ...int) *Snappy {
	return &Snappy{maxSize: bodyLimit(maxSize)}
}

func (s *Snappy) Zip(_ uint32, body []byte) ([]byte, error) {
	if err := checkSize(len(body), s.maxSize); err != nil {
		return nil, err
	}
	return snappy.Encode(nil, body), nil
}

func (s *Snappy) Unzip(_ uint32, body []byte) ([]byte, error) {
	size, err := snappy.DecodedLen(body)
	if err != nil {
		return nil, err
	}
	if err := checkSize(size, s.maxSize); err != nil {
		return nil, err
	}
	return snappy.Decode(make([]byte, 0, size), body)
}
