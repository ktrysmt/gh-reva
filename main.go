package main

import (
	"os"

	"github.com/ktrysmt/gh-rv/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		cmd.PrintError(err)
		os.Exit(1)
	}
}
