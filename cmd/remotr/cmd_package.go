package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
)

func packageCommand() *cli.Command {
	return &cli.Command{
		Name:        "package",
		Usage:       "scaffold and build custom app packages",
		Category:    catConfig,
		Description: "Create local package source trees, build zip archives, and optionally publish them.",
		Commands: []*cli.Command{
			packageCreateCommand(),
			packageBuildCommand(),
		},
	}
}

func packageCreateCommand() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create a package source directory with scaffold files",
		Description: withExamples(`Creates remotr-package.yaml and starter files for binary, script, or build install modes.`,
			"remotr package create --path ./mycli",
			"remotr package create --path ./tools/yq --name internal/yq --version 1.0.0 --mode script"),
		Action: actionPackageCreate,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "path", Required: true, Usage: "directory to create"},
			&cli.StringFlag{Name: "name", Usage: "catalog package name (default: final path segment)"},
			&cli.StringFlag{Name: "version", Usage: "package version (default: 0.1.0)"},
			&cli.StringFlag{Name: "mode", Value: "binary", Usage: "install mode: binary, script, or build"},
			&cli.BoolFlag{Name: "force", Usage: "reuse an existing directory"},
		},
	}
}

func packageBuildCommand() *cli.Command {
	return &cli.Command{
		Name:  "build",
		Usage: "validate a package directory and write a zip archive",
		Description: withExamples(`Validates remotr-package.yaml and referenced files, then writes a zip.`,
			"remotr package build --path ./mycli",
			"remotr package build --path ./mycli --output ./dist/mycli-1.0.0.zip",
			"remotr package build --path ./mycli --push"),
		Action: actionPackageBuild,
		Flags: append(outputFlags(),
			&cli.StringFlag{Name: "path", Required: true, Usage: "package source directory"},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "zip output path (default: <name>-<version>.zip in cwd)"},
			&cli.BoolFlag{Name: "push", Usage: "upload to S3 and register with the server after building"},
			&cli.StringFlag{Name: "s3-key", Usage: "override S3 object key when using --push"},
		),
	}
}

func actionPackageCreate(_ context.Context, c *cli.Command) error {
	dir := strings.TrimSpace(c.String("path"))
	manifest, err := apppackages.CreateScaffold(apppackages.ScaffoldOptions{
		Dir:     dir,
		Name:    c.String("name"),
		Version: c.String("version"),
		Mode:    c.String("mode"),
		Force:   c.Bool("force"),
	})
	if err != nil {
		return exitErr(1, "package create: %v", err)
	}
	if c.Bool("json") {
		return encodeJSON(map[string]any{
			"path":    dir,
			"name":    manifest.Name,
			"version": manifest.Version,
			"mode":    manifest.Install.Mode,
		})
	}
	fmt.Printf("created package source at %s\n", dir)
	fmt.Printf("name: %s\n", manifest.Name)
	fmt.Printf("version: %s\n", manifest.Version)
	fmt.Printf("mode: %s\n", manifest.Install.Mode)
	fmt.Printf("next: edit files, then run remotr package build --path %s\n", dir)
	return nil
}

func actionPackageBuild(ctx context.Context, c *cli.Command) error {
	dir := strings.TrimSpace(c.String("path"))
	data, sum, err := apppackages.BuildZipFromDir(dir)
	if err != nil {
		return exitErr(1, "package build: %v", err)
	}

	out := strings.TrimSpace(c.String("output"))
	if out == "" {
		out = apppackages.DefaultZipFilename(sum.Manifest)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil && filepath.Dir(out) != "." {
		return exitErr(1, "package build: %v", err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil { // #nosec G703
		return exitErr(1, "package build: write zip: %v", err)
	}

	var published admin.AppPackage
	if c.Bool("push") {
		settings, err := resolveSettings(c)
		if err != nil {
			return exitErr(2, "package build: %v", err)
		}
		if err := requireOperatorCLI(settings, "package build --push"); err != nil {
			return err
		}
		key := strings.TrimSpace(c.String("s3-key"))
		if key == "" {
			key = apppackages.DefaultS3Key(sum.Manifest.Name, sum.Manifest.Version)
		}
		published, err = publishAppPackage(ctx, c, settings, data, sum, key)
		if err != nil {
			return err
		}
	}

	if c.Bool("json") {
		payload := map[string]any{
			"name":    sum.Manifest.Name,
			"version": sum.Manifest.Version,
			"sha256":  sum.SHA256,
			"bytes":   sum.Size,
			"output":  out,
		}
		if c.Bool("push") {
			payload["published"] = published
		}
		return encodeJSON(payload)
	}

	fmt.Printf("built %s@%s\n", sum.Manifest.Name, sum.Manifest.Version)
	fmt.Printf("output: %s\n", out)
	fmt.Printf("sha256: %s\n", sum.SHA256)
	fmt.Printf("bytes: %d\n", sum.Size)
	if c.Bool("push") {
		fmt.Printf("published to catalog as %s@%s\n", published.Name, published.Version)
		fmt.Printf("s3_key: %s\n", published.S3Key)
	}
	return nil
}

func publishAppPackage(
	ctx context.Context,
	c *cli.Command,
	settings opconfig.Settings,
	data []byte,
	_ apppackages.ZipSummary,
	s3Key string,
) (admin.AppPackage, error) {
	client, err := newAdminClient(settings)
	if err != nil {
		return admin.AppPackage{}, exitErr(1, "publish: %v", err)
	}
	var rec admin.AppPackage
	err = runWithSpinner(ctx, c, "uploading app package", func(ctx context.Context) error {
		r, upErr := client.UploadAppPackage(data, s3Key)
		if upErr != nil {
			return upErr
		}
		rec = r
		return nil
	})
	if err != nil {
		return admin.AppPackage{}, apiErr(c, "publish", err)
	}
	return rec, nil
}
