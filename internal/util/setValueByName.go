package util

import (
	"reflect"
)

func SetValueByName(v interface{}, field string, newval interface{}) {
	r := reflect.ValueOf(v).Elem().FieldByName(field)
	if reflect.TypeOf(newval).Kind() == reflect.Slice {
		switch newval.(type) {
		case []int64:
			r.Set(reflect.ValueOf(newval.([]int64)))
		case []interface{}:
			intArray := ArrToInt64(newval.([]interface{}))
			r.Set(reflect.ValueOf(intArray))
		}
		return
	}
	r.Set(reflect.ValueOf(newval))

}
