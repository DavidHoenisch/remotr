package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/urfave/cli/v3"
)

func actionConfigRender(_ context.Context, c *cli.Command) error {
	dir := c.Args().First()
	if dir == "" {
		if root := findConfigRepoRoot("."); root != "" {
			dir = root
		} else {
			dir = "."
		}
	}
	if c.NArg() > 1 {
		return exitErr(2, "config render: unexpected arguments")
	}

	fleet := strings.TrimSpace(c.String("fleet"))
	endpoint := strings.TrimSpace(c.String("endpoint"))
	if fleet != "" && endpoint != "" {
		return exitErr(2, "config render: use only one of --fleet and --endpoint")
	}

	var desired, crons []byte
	var desiredDigest, cronsDigest string
	var label string
	var err error

	switch {
	case endpoint != "":
		label = "endpoints/" + endpoint
		desired, crons, desiredDigest, cronsDigest, err = configcompose.RenderEndpoint(dir, endpoint)
	case fleet != "":
		label = "fleets/" + fleet
		desired, crons, desiredDigest, cronsDigest, err = configcompose.RenderFleet(dir, fleet)
	default:
		artifacts, renderErr := configcompose.RenderAll(dir)
		if renderErr != nil {
			return exitErr(1, "config render: %v", renderErr)
		}
		return printRenderedArtifacts(c, artifacts)
	}
	if err != nil {
		return exitErr(1, "config render: %v", err)
	}

	artifacts := []configcompose.RenderedArtifact{
		{TargetType: "render", TargetID: label, ArtifactType: "desired", YAML: desired, Digest: desiredDigest},
	}
	if len(crons) > 0 {
		artifacts = append(artifacts, configcompose.RenderedArtifact{
			TargetType:   "render",
			TargetID:     label,
			ArtifactType: "crons",
			YAML:         crons,
			Digest:       cronsDigest,
		})
	}
	return printRenderedArtifacts(c, artifacts)
}

func printRenderedArtifacts(c *cli.Command, artifacts []configcompose.RenderedArtifact) error {
	outPath := strings.TrimSpace(c.String("output"))
	format := resolveFormat(c)

	if outPath != "" && len(artifacts) != 1 {
		return exitErr(2, "config render: --output requires a single artifact (--fleet or --endpoint)")
	}

	if outPath != "" {
		if err := os.WriteFile(outPath, artifacts[0].YAML, 0o644); err != nil {
			return exitErr(1, "config render: %v", err)
		}
		fmt.Printf("wrote %s\n", outPath)
		return nil
	}

	if format == formatJSON {
		type jsonArtifact struct {
			Target  string `json:"target"`
			Type    string `json:"artifactType"`
			Digest  string `json:"digest"`
			Content string `json:"content"`
		}
		out := make([]jsonArtifact, 0, len(artifacts))
		for _, a := range artifacts {
			out = append(out, jsonArtifact{
				Target:  a.TargetID,
				Type:    a.ArtifactType,
				Digest:  a.Digest,
				Content: string(a.YAML),
			})
		}
		return encodeJSON(out)
	}

	for i, a := range artifacts {
		if len(artifacts) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("--- %s (%s) digest=%s ---\n", a.TargetID, a.ArtifactType, a.Digest)
		}
		fmt.Print(string(a.YAML))
		if len(a.YAML) > 0 && a.YAML[len(a.YAML)-1] != '\n' {
			fmt.Println()
		}
	}
	return nil
}
