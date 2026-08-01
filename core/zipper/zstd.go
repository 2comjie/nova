package zipper

import (
	"runtime"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type Zstd struct {
	encoder   *zstd.Encoder
	decoder   *zstd.Decoder
	maxSize   int
	closeOnce sync.Once
}

func NewZstd(maxSize ...int) (*Zstd, error) {
	limit := bodyLimit(maxSize)
	concurrency := runtime.GOMAXPROCS(0)

	encoder, err := zstd.NewWriter(
		nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(concurrency),
		zstd.WithLowerEncoderMem(true),
		zstd.WithWindowSize(1<<20),
		zstd.WithZeroFrames(true),
	)
	if err != nil {
		return nil, err
	}
	decoder, err := zstd.NewReader(
		nil,
		zstd.WithDecoderConcurrency(concurrency),
		zstd.WithDecoderLowmem(true),
		zstd.WithDecoderMaxMemory(uint64(limit)),
		zstd.WithDecodeAllCapLimit(true),
	)
	if err != nil {
		encoder.Close()
		return nil, err
	}
	return &Zstd{
		encoder: encoder,
		decoder: decoder,
		maxSize: limit,
	}, nil
}

func (z *Zstd) Zip(_ uint32, body []byte) ([]byte, error) {
	if err := checkSize(len(body), z.maxSize); err != nil {
		return nil, err
	}
	return z.encoder.EncodeAll(body, nil), nil
}

func (z *Zstd) Unzip(_ uint32, body []byte) ([]byte, error) {
	output, err := z.decoder.DecodeAll(body, nil)
	if err != nil {
		return nil, err
	}
	if err := checkSize(len(output), z.maxSize); err != nil {
		return nil, err
	}
	return output, nil
}

func (z *Zstd) Close() {
	z.closeOnce.Do(func() {
		z.encoder.Close()
		z.decoder.Close()
	})
}
