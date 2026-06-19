//go:build openbsd

package unveil

import "golang.org/x/sys/unix"

func Unveil(uploadDirs ...string) error {
	for _, d := range uploadDirs {
		if err := unix.Unveil(d, "rwc"); err != nil {
			return err
		}
	}

	return unix.UnveilBlock()
}
