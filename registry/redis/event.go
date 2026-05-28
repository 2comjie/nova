package redis

import "github.com/2comjie/wali/core/endpoint"

type EventType string

const (
	EventRegister   EventType = "register"
	EventDeregister EventType = "deregister"
	EventUpdateMeta EventType = "updateMeta"
	EventDeleteMeta EventType = "deleteMeta"
)

type UpdateEvent struct {
	Type     EventType                `json:"type"`
	Instance endpoint.ServiceInstance `json:"instance"`
}
