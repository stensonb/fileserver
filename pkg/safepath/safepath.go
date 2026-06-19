package safepath

import (
	"fmt"
	"path/filepath"
)

type TooManyConsecutiveDotsErr struct {}

var _ error = &TooManyConsecutiveDotsErr{}

func (m TooManyConsecutiveDotsErr) Error() string {
	return "too many consecutive dots"
}

type TooManyFileSeparatorsErr struct {
	fsCount uint
}

var _ error = &TooManyFileSeparatorsErr{}

func (m TooManyFileSeparatorsErr) Error() string {
	return fmt.Sprintf("too many file separators (%v)", m.fsCount)
}

type BadCharactersFoundErr struct{}

var _ error = &BadCharactersFoundErr{}

func (m BadCharactersFoundErr) Error() string {
	return "prohibited characters found"
}

func Clean(input string) (string, error) {
	// https://github.com/stensonb/fileserver/security/code-scanning/2

	// Do not rely on simply replacing problematic sequences such as "../". For example, after applying this filter to ".../...//", the resulting string would still be "../".
	input = filepath.Clean(input)

	// Do not allow more than a single "." character.
	// Do not allow directory separators such as "/" or "\" (depending on the file system).

	var lastRuneDot bool
	var fsCount uint = 0
	dotRune := []rune(".")[0]

	for _, r := range input {
		switch r {
		case dotRune:
			if lastRuneDot {
				return "", TooManyConsecutiveDotsErr{}
			}
			lastRuneDot = true
		case filepath.Separator:
			fsCount++
		default:
			lastRuneDot = false
		}
	}

	if fsCount > 0 {
		return "", TooManyFileSeparatorsErr{fsCount}
	}

	// Use an allowlist of known good patterns.
	// where "*" is "any sequence of non-separator characters"
	matched, err := filepath.Match("*", input)
	if err != nil {
		return "", err
	} else if !matched {
		return "", BadCharactersFoundErr{}
	}

	// got here, so the input matched without error
	return input, nil
}
