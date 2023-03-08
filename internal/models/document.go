package models

type Document struct {
	Id        int64   `reindex:"id,,pk" json:"Id"`
	ParentId  int64   `reindex:"parent_id,,sparse" json:"ParentId"`
	Depth     int     `reindex:"depth" json:"Depth"`
	Body      string  `reindex:"body" json:"Body"`
	ChildList []int64 `reindex:"child_list,,sparse" json:"ChildList"`
}
