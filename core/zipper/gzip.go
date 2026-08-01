package zipper

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
)

type Gzip struct {
	maxSize int
	writers sync.Pool
}

func NewGzip(maxSize ...int) *Gzip {
	g := &Gzip{maxSize: bodyLimit(maxSize)}
	g.writers.New = func() any {
		writer, err := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		if err != nil {
			panic(err)
		}
		return writer
	}
	return g
}

func (g *Gzip) Zip(_ uint32, body []byte) ([]byte, error) {
	if err := checkSize(len(body), g.maxSize); err != nil {
		return nil, err
	}

	var output bytes.Buffer
	writer := g.writers.Get().(*gzip.Writer)
	writer.Reset(&output)
	_, writeErr := writer.Write(body)
	closeErr := writer.Close()
	writer.Reset(io.Discard)
	g.writers.Put(writer)
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return output.Bytes(), nil
}

func (g *Gzip) Unzip(_ uint32, body []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	output, readErr := io.ReadAll(io.LimitReader(reader, int64(g.maxSize)+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := checkSize(len(output), g.maxSize); err != nil {
		return nil, err
	}
	return output, nil
}
