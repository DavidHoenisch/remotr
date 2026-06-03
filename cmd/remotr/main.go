package main

import (
	"os"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(runApp())
}
