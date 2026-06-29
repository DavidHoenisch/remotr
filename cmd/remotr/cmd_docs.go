package main

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/urfave/cli/v3"
)

const remotrDocsURL = "https://davidhoenisch.github.io/remotr"

var lookPath = exec.LookPath

func docsCommand() *cli.Command {
	return &cli.Command{
		Name:  "docs",
		Usage: "open Remotr documentation in the default browser",
		Description: withExamples(`Opens the published documentation site in your default browser via xdg-open.
When xdg-open is unavailable or cannot launch a browser, prints the URL to stdout.`,
			"remotr docs"),
		Action: actionDocs,
	}
}

func actionDocs(_ context.Context, _ *cli.Command) error {
	if opened, err := openURLInBrowser(remotrDocsURL); opened && err == nil {
		return nil
	}
	fmt.Println(remotrDocsURL)
	return nil
}

func openURLInBrowser(url string) (opened bool, err error) {
	opener, err := lookPath("xdg-open")
	if err != nil {
		return false, nil
	}
	if err := exec.Command(opener, url).Run(); err != nil {
		return false, err
	}
	return true, nil
}
