package models

type Document struct {
	Id        int64   `reindex:"id,,pk"`
	ParentId  int64   `reindex:"parent_id"`
	Body      string  `reindex:"body"`
	ChildList []int64 `reindex:"child_list"`
}
