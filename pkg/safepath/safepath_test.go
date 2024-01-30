package safepath

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafePath(t *testing.T) {
	cases := map[string]struct {
		output        string
		expectedError error
	}{
		"good": {
			output:        "good",
			expectedError: nil,
		},
		"fine.foo": {
			output:        "fine.foo",
			expectedError: nil,
		},
		"bad.morethanone.dot": {
			output:        "",
			expectedError: &TooManyDotsErr{},
		},
		fmt.Sprintf("bad%smorethanzeroslashes", string(filepath.Separator)): {
			output:        "",
			expectedError: &TooManyFileSeparatorsErr{},
		},
		"some-other-chars-œ-Ÿ-¥-ç": {
			output:        "some-other-chars-œ-Ÿ-¥-ç",
			expectedError: nil,
		},
	}

	for input, tc := range cases {
		output, err := Clean(input)

		if tc.expectedError != nil {
			require.ErrorAs(t, err, tc.expectedError)
		} else {
			require.NoError(t, err)
		}

		require.Equal(t, tc.output, output)
	}
}
