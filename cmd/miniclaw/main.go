package main

import (
	"os"

	"miniclaw2/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
