package main

import (
	"os"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestMain(m *testing.M) {
	cli.OsExiter = func(int) {}
	os.Exit(m.Run())
}
