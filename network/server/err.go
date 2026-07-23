package netServer

import "errors"

var ErrConnNotFound = errors.New("conn not found")
var ErrConnBound = errors.New("conn bound")
var ErrWriteChannelFull = errors.New("write channel full")
var ErrConnClosed = errors.New("conn closed")
