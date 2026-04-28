package diff

import (
	godiff "github.com/sourcegraph/go-diff/diff"
)

type FileDiff = godiff.FileDiff
type Hunk = godiff.Hunk

func Parse(patch string) ([]*FileDiff, error) {
	return godiff.ParseMultiFileDiff([]byte(patch))
}
