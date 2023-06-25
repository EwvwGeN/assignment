package models

type Document struct {
	Id        int64   `reindex:"id,,pk" json:"Id"`
	ParentId  int64   `reindex:"parent_id,,sparse" json:"ParentId"`
	Depth     int     `reindex:"depth" json:"Depth"`
	Sort      int     `reindexer:"sort" json:"Sort"`
	Body      string  `reindex:"body" json:"Body"`
	ChildList []int64 `reindex:"child_list,,sparse" json:"ChildList"`
}

func (doc *Document) DeepCopy() interface{} {
	copyItem := &Document{
		Id:        doc.Id,
		ParentId:  doc.ParentId,
		Depth:     doc.Depth,
		Sort:      doc.Sort,
		Body:      doc.Body,
		ChildList: make([]int64, cap(doc.ChildList), len(doc.ChildList)),
	}
	copy(copyItem.ChildList, doc.ChildList)
	return copyItem
}
