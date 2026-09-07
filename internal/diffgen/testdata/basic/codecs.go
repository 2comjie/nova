//go:build diff_fast

package basic

type CodecTags struct {
	Uid      uint64 `diff:"1" json:"uid" bson:"_id"`
	Omitted  string `diff:"2" json:"omitted,omitempty" bson:"omitted,omitempty"`
	JSONOnly int32  `diff:"3" json:"json_only" bson:"-"`
	BSONOnly int32  `diff:"4" json:"-" bson:"bson_only"`
	Hidden   int32  `diff:"5" json:"-" bson:"-"`
	Runtime  string `diff:"-" json:"runtime" bson:"runtime"`
}
