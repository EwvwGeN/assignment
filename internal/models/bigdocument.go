package models

type BigDocument struct {
	Id        int64         `json:"Id"`
	Sort      int           `json:"Sort"`
	Body      string        `json:"Body"`
	ChildList []BigDocument `json:"ChildList"`
}
