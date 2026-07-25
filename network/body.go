package network

// Zipper 由业务层决定具体压缩算法以及哪些 route 需要压缩.
type Zipper interface {
	Zip(route uint32, body []byte) ([]byte, error)
	// Unzip 的实现必须在分配大内存前限制解压结果，最终结果还会再受 WithMaxBody 校验。
	Unzip(route uint32, body []byte) ([]byte, error)
}

// Cryptor 只加密和解密业务包的 Body，不处理 packet 包头。
type Cryptor interface {
	Encrypt(route uint32, seq uint64, body []byte) ([]byte, error)
	Decrypt(route uint32, seq uint64, body []byte) ([]byte, error)
}
