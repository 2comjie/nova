package network

import (
	"fmt"

	"github.com/2comjie/wali/packet"
)

func encodeBody(options options, messageType packet.Type, route uint32, seq uint64, body []byte) ([]byte, error) {
	if len(body) > options.maxBody {
		return nil, fmt.Errorf("network: Body超过上限: %d", len(body))
	}
	if messageType != packet.Req && messageType != packet.Rsp && messageType != packet.Push {
		return body, nil
	}

	var err error
	if options.zipper != nil {
		body, err = options.zipper.Zip(route, body)
		if err != nil {
			return nil, err
		}
	}
	if options.cryptor != nil {
		body, err = options.cryptor.Encrypt(route, seq, body)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func decodeBody(options options, message *packet.Message) ([]byte, error) {
	body := message.Body
	var err error
	if options.cryptor != nil {
		body, err = options.cryptor.Decrypt(message.Route, message.Seq, body)
		if err != nil {
			return nil, err
		}
	}
	if options.zipper != nil {
		body, err = options.zipper.Unzip(message.Route, body)
		if err != nil {
			return nil, err
		}
	}
	if len(body) > options.maxBody {
		return nil, fmt.Errorf("network: Body超过上限: %d", len(body))
	}
	return body, nil
}
