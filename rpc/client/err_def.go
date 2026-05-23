package client

import "errors"

var ErrInvalidTarget = errors.New("invalid target")
var ErrServiceNotFound = errors.New("rpc client: service not found")
