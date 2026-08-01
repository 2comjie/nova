package network

type Auther interface {
	Auth(token []byte) (uid string, err error)
}

type AuthFunc func(token []byte) (uid string, err error)

func (f AuthFunc) Auth(token []byte) (string, error) {
	return f(token)
}
