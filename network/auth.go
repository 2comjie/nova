package network

type Auther interface {
	Auth(token []byte) (uid uint64, err error)
}

type AuthFunc func(token []byte) (uid uint64, err error)

func (f AuthFunc) Auth(token []byte) (uint64, error) {
	return f(token)
}
