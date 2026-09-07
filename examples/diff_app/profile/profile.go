//go:build diff_fast

package profile

import "time"

type State int32
type StateAlias = State
type PlayerId = uint64

const (
	Offline State = 0
	Online  State = 1
)

type Profile struct {
	Id        PlayerId             `diff:"1" json:"id" bson:"_id"`
	State     StateAlias           `diff:"2" json:"state" bson:"state"`
	LoginAt   time.Time            `diff:"3" json:"login_at" bson:"login_at"`
	Cooldown  time.Duration        `diff:"4" json:"cooldown" bson:"cooldown"`
	History   []State              `diff:"5" json:"history" bson:"history"`
	RewardsAt map[string]time.Time `diff:"6" json:"rewards_at" bson:"rewards_at"`
}
