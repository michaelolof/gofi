package utils

import "strings"

func Pop[T any](arr *[]T) {
	if arr == nil {
		return
	}

	if len(*arr) == 0 {
		return
	}

	var sb = *arr
	*arr = sb[:len(sb)-1]
}

func Append[T any](arr *[]T, val T) {
	*arr = append(*arr, val)
}

func Push[T comparable](arr *[]T, val T) []T {
	var d T
	if arr == nil && val != d {
		return []T{val}
	} else if arr != nil && val == d {
		return *arr
	} else {
		return append(*arr, val)
	}
}

func LastItem[T any](arr *[]T) *T {
	var t T
	if arr == nil {
		return &t
	}

	if len(*arr) == 0 {
		return &t
	}

	return &(*arr)[len(*arr)-1]
}

func UpdateItem[T any](arr *[]T, fn func(b *T)) {
	if arr == nil {
		return
	}

	if len(*arr) == 0 {
		return
	}

	fn(&(*arr)[len(*arr)-1])
}

func ToUpperFirst(s string) string {
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
