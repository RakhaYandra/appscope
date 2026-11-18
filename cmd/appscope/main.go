package main

import (
	"fmt"
	"os"

	"github.com/RakhaYandra/appscope/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "appscope:", err)
		os.Exit(1)
	}
}
