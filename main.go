package main

import (
	"os"

	"github.com/kaosb/tgcli/cmd"
)

func main() {
	if err := cmd.Execute(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
