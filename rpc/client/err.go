package client

import "errors"

var ErrInvalidTarget = errors.New("invalid target")
var ErrNoAnyService = errors.New("no any service")
var ErrLocatorUnavailable = errors.New("locator unavailable")
var ErrInvalidBalancePolicy = errors.New("invalid balance policy")
