package zipper

import "errors"

const DefaultMaxSize = 8 << 20

var ErrBodyTooLarge = errors.New("zipper: Body超过上限")

func bodyLimit(limit []int) int {
	if len(limit) > 0 && limit[0] > 0 {
		return limit[0]
	}
	return DefaultMaxSize
}

func checkSize(size int, limit int) error {
	if size <= limit {
		return nil
	}
	return ErrBodyTooLarge
}
