package network

import "errors"

var ErrConnHanged = errors.New("conn hanged")
var ErrConnClosed = errors.New("conn closed")
var ErrConnNotOpen = errors.New("conn not open")
var ErrConnNotHanged = errors.New("conn not hanged")
var ErrWritChFull = errors.New("conn write chan full")
var ErrTooManyConn = errors.New("too may conns")
var ErrCallTimeout = errors.New("call timeout")
var ErrConnNotFound = errors.New("conn not found")
var ErrInvalidUID = errors.New("invalid uid")
