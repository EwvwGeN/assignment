package models

type AllowedField struct {
	Body      string  `reindex:"body" json:"Body"`
	ChildList []int64 `reindex:"child_list,,sparse" json:"ChildList"`
}
