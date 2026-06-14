package logdef

import "io"

type MultiWriter struct {
	writers []io.Writer
}

func NewMultiWriter() *MultiWriter {
	m := &MultiWriter{
		writers: nil,
	}
	return m
}

func (m *MultiWriter) Write(p []byte) (n int, err error) {
	for _, oneWriter := range m.writers {
		n, err = oneWriter.Write(p)
		if err != nil {
			return
		}
	}
	return
}

func (m *MultiWriter) AddWriter(writer io.Writer) {
	m.writers = append(m.writers, writer)

}
