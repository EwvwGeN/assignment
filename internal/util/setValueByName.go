package util

import (
	"reflect"
)

func SetValueByName(v interface{}, field string, newval interface{}) {
	r := reflect.ValueOf(v).Elem().FieldByName(field)
	if reflect.TypeOf(newval).Kind() == reflect.Slice {
		intArray := ArrToInt64(newval.([]interface{}))
		r.Set(reflect.ValueOf(intArray))
		return
	}
	r.Set(reflect.ValueOf(newval))

}
