// main.go
package main

import (
	"os"

	"github.com/boneskull/gh-stack/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
