package models

type BigDocument struct {
	Id        int64         `json:"Id"`
	Body      string        `json:"Body"`
	ChildList []BigDocument `json:"ChildList"`
}
