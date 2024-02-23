package twilio

func max[K int8 | int16 | int32 | int64 | int | uint8 | uint16 | uint32 | uint64 | float32 | float64](args ...K) K {
	m := args[0]
	for _, v := range args {
		if v > m {
			m = v
		}
	}
	return m
}

func toPtr[K any](val K) *K {
	return &val
}

func toValue[K any](val *K) K {
	if val == nil {
		return *new(K)
	}
	return *val
}
