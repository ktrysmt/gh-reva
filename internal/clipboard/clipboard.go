package clipboard

import "github.com/atotto/clipboard"

func Yank(s string) error {
	return clipboard.WriteAll(s)
}
