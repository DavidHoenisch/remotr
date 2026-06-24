package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
)

func appCommand() *cli.Command {
	return &cli.Command{
		Name:        "app",
		Usage:       "manage custom app packages",
		Category:    catConfig,
		Description: "Publish and manage zip-based custom applications for endpoint deployment.",
		Commands: []*cli.Command{
			appPackageCommand(),
			appPublishCommand(),
			appListCommand(),
			appShowCommand(),
			appDeleteCommand(),
		},
	}
}

func appPackageCommand() *cli.Command {
	return &cli.Command{
		Name:  "package",
		Usage: "inspect package archives",
		Commands: []*cli.Command{
			appPackageValidateCommand(),
		},
	}
}

func appPackageValidateCommand() *cli.Command {
	return &cli.Command{
		Name:      "validate",
		Usage:     "validate a package zip",
		ArgsUsage: "PATH.zip",
		Description: withExamples("",
			"remotr app package validate ./mycli-1.0.0.zip"),
		Action: actionAppPackageValidate,
	}
}

func appPublishCommand() *cli.Command {
	return &cli.Command{
		Name:      "publish",
		Usage:     "upload a package zip to S3 and register it",
		ArgsUsage: "PATH.zip",
		Description: withExamples(`Requires S3 env (REMOTR_S3_BUCKET or BUCKET_NAME, AWS credentials) and operator credentials.`,
			"remotr app publish ./mycli-1.0.0.zip",
			"remotr app publish ./tool.zip --s3-key custom/path/tool.zip"),
		Action: actionAppPublish,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "s3-key", Usage: "override S3 object key (default: app-packages/<name>/<version>/...)"},
		},
	}
}

func appListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list published app packages",
		Description: withExamples("",
			"remotr app list", "remotr app list --name internal/ --json"),
		Action: actionAppList,
		Flags: append(outputFlags(),
			&cli.StringFlag{Name: "name", Usage: "filter by package name prefix"},
		),
	}
}

func appShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "show package metadata",
		ArgsUsage: "NAME VERSION",
		Description: withExamples("",
			`remotr app show internal/mycli 1.0.0`),
		Action: actionAppShow,
	}
}

func appDeleteCommand() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "remove a package from the catalog",
		ArgsUsage: "NAME VERSION",
		Description: withExamples(`Requires --confirm matching "NAME VERSION".`,
			`remotr app delete internal/mycli 1.0.0 --confirm "internal/mycli 1.0.0"`),
		Action: actionAppDelete,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "delete-object", Usage: "also delete the S3 object"},
			confirmFlag("name and version"),
		},
	}
}

func actionAppPackageValidate(_ context.Context, c *cli.Command) error {
	path, err := resolveZipPath(c)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path) // #nosec G703 -- operator-supplied path
	if err != nil {
		return exitErr(1, "app validate: %v", err)
	}
	sum, err := apppackages.ValidateZip(data)
	if err != nil {
		return exitErr(1, "app validate: %v", err)
	}
	if c.Bool("json") {
		return encodeJSON(map[string]any{
			"ok":      true,
			"name":    sum.Manifest.Name,
			"version": sum.Manifest.Version,
			"sha256":  sum.SHA256,
			"bytes":   sum.Size,
		})
	}
	fmt.Printf("ok: %s@%s sha256=%s bytes=%d\n", sum.Manifest.Name, sum.Manifest.Version, sum.SHA256, sum.Size)
	return nil
}

func actionAppPublish(ctx context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "app publish: %v", err)
	}
	if err := requireOperatorCLI(settings, "app publish"); err != nil {
		return err
	}
	path, err := resolveZipPath(c)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path) // #nosec G703
	if err != nil {
		return exitErr(1, "app publish: %v", err)
	}
	sum, err := apppackages.ValidateZip(data)
	if err != nil {
		return exitErr(1, "app publish: %v", err)
	}

	key := strings.TrimSpace(c.String("s3-key"))
	if key == "" {
		key = apppackages.DefaultS3Key(sum.Manifest.Name, sum.Manifest.Version)
	}

	rec, err := publishAppPackage(ctx, c, settings, data, sum, key)
	if err != nil {
		return err
	}
	if c.Bool("json") {
		return encodeJSON(rec)
	}
	fmt.Printf("published %s@%s\n", rec.Name, rec.Version)
	fmt.Printf("s3_key: %s\n", rec.S3Key)
	fmt.Printf("sha256: %s\n", rec.SHA256)
	return nil
}

func actionAppList(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "app list: %v", err)
	}
	if err := requireOperatorCLI(settings, "app list"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "app list: %v", err)
	}
	items, err := client.ListAppPackages(c.String("name"))
	if err != nil {
		return apiErr(c, "app list", err)
	}
	if c.Bool("json") {
		return encodeJSON(items)
	}
	if len(items) == 0 {
		fmt.Println("no app packages")
		return nil
	}
	for _, item := range items {
		fmt.Printf("%s@%s  sha256=%s  created=%s\n",
			item.Name, item.Version, item.SHA256, item.CreatedAt.UTC().Format(time.RFC3339))
	}
	return nil
}

func actionAppShow(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "app show: %v", err)
	}
	name, version, err := resolveAppNameVersion(c)
	if err != nil {
		return err
	}
	if err := requireOperatorCLI(settings, "app show"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "app show: %v", err)
	}
	item, err := client.GetAppPackage(name, version)
	if err != nil {
		return apiErr(c, "app show", err)
	}
	if c.Bool("json") {
		return encodeJSON(item)
	}
	fmt.Printf("name: %s\n", item.Name)
	fmt.Printf("version: %s\n", item.Version)
	fmt.Printf("s3_key: %s\n", item.S3Key)
	fmt.Printf("sha256: %s\n", item.SHA256)
	fmt.Printf("install_mode: %s\n", item.Manifest.Install.Mode)
	fmt.Printf("created: %s\n", item.CreatedAt.UTC().Format(time.RFC3339))
	return nil
}

func actionAppDelete(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "app delete: %v", err)
	}
	name, version, err := resolveAppNameVersion(c)
	if err != nil {
		return err
	}
	confirm := strings.TrimSpace(c.String("confirm"))
	if confirm != name+" "+version {
		return exitErr(2, "app delete: --confirm must match %q", name+" "+version)
	}
	if err := requireOperatorCLI(settings, "app delete"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "app delete: %v", err)
	}
	if err := client.DeleteAppPackage(name, version, c.Bool("delete-object")); err != nil {
		return apiErr(c, "app delete", err)
	}
	fmt.Printf("deleted %s@%s\n", name, version)
	return nil
}

func resolveZipPath(c *cli.Command) (string, error) {
	if c.NArg() != 1 {
		return "", exitErr(2, "app: exactly one zip path required")
	}
	return c.Args().First(), nil
}

func resolveAppNameVersion(c *cli.Command) (string, string, error) {
	if c.NArg() != 2 {
		return "", "", exitErr(2, "app: NAME and VERSION required")
	}
	return c.Args().Get(0), c.Args().Get(1), nil
}
