package main

import (
	"os"

	"secrio/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.RealDeps(), os.Args[1:]))
}
