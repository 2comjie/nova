package util

import "io"

func WriteFull(w io.Writer, p []byte) (int64, error) {
	written := int64(0)
	target := int64(len(p))
	for written < target {
		cur, err := w.Write(p[written:])
		if err != nil {
			return written, err
		}
		written += int64(cur)
	}
	return written, nil
}
