package diff

import "github.com/spf13/cast"

type BoolKey bool

func (key BoolKey) MarshalText() ([]byte, error) {
	return []byte(cast.ToString(bool(key))), nil
}

func (key *BoolKey) UnmarshalText(data []byte) error {
	value, err := cast.ToBoolE(string(data))
	if err != nil {
		return err
	}
	*key = BoolKey(value)
	return nil
}
