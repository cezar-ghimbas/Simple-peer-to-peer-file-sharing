package util

type Pair struct {
	Values [2]interface{}
}

func CreatePair(value1, value2 interface{}) Pair {
	return Pair{Values: [2]interface{}{value1, value2}}
}
