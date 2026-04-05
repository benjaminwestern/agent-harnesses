package main

import (
	"fmt"
	"os"

	"github.com/benjaminwestern/agentic-control/internal/harness"
)

func main() {
	if err := harness.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
