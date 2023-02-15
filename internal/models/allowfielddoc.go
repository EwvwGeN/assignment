package models

type AllowedField struct {
	Id        int64   `reindex:"id,,pk" json:"Id"`
	Body      string  `reindex:"body" json:"Body"`
	ChildList []int64 `reindex:"child_list,,sparse" json:"ChildList"`
}
