//go:build !openbsd

package unveil

func Unveil(string) error { return nil }
