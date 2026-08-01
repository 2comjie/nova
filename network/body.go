package network

type Zipper interface {
	Zip(route uint32, body []byte) ([]byte, error)
	Unzip(route uint32, body []byte) ([]byte, error)
}

type Cryptor interface {
	Encrypt(route uint32, seq uint64, body []byte) ([]byte, error)
	Decrypt(route uint32, seq uint64, body []byte) ([]byte, error)
}
