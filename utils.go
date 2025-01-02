package gofi

func fallback[T comparable](v T, d T) T {
	var e T
	if v == e {
		return d
	} else {
		return v
	}
}
