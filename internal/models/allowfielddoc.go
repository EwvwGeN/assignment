package models

type AllowedField struct {
	Sort      int     `reindexer:"sort" json:"Sort"`
	Body      string  `reindex:"body" json:"Body"`
	ChildList []int64 `reindex:"child_list,,sparse" json:"ChildList"`
}
