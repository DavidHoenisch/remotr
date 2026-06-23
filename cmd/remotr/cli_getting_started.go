package main

import (
	"fmt"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
)

func printGettingStarted() {
	fmt.Print(`Remotr operator CLI — common first steps:

  1. Bootstrap operator credentials (one-time):
     remotr bootstrap --server-url https://remotr.example:8443 --ca ca.crt --token TOKEN

  2. Create an enrollment token for a fleet:
     remotr enroll token create --fleet engineering

  3. List enrolled endpoints:
     remotr endpoint list

  Diagnose setup:
     remotr doctor

  Operator config: ` + opconfig.DefaultPath() + `
  Guide: docs/guides/operator-workflows.md

  Run remotr --help or remotr <command> --help for details.
`)
}
