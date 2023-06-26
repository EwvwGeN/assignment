package util

func ArrToInt64(in []interface{}) (out []int64) {
	out = make([]int64, 0, len(in))
	for _, v := range in {
		out = append(out, int64(v.(float64)))
	}
	return
}
