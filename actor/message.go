package actor

type Message struct {
	Route uint32
	Body  []byte
}

type ActivationPolicy uint32

const (
	ActivationLoad    ActivationPolicy = 1 // 仅加载
	ActivationIgnore  ActivationPolicy = 2 // 忽略不存在的actor
	ActivationRequire ActivationPolicy = 3 // 必须要有actor
)
