package packet

import "errors"

var ErrInvalidPacket = errors.New("invalid packet")
var ErrPacketSizeTooLong = errors.New("packet size too long")
