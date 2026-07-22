package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/sys/unix"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func secretCommand() *cli.Command {
	return &cli.Command{
		Name: "secret", Category: catSecurity, Usage: "manage encrypted Remotr secret versions",
		Commands: []*cli.Command{
			{
				Name: "upload", Usage: "upload an inactive encrypted secret version", ArgsUsage: "<logical-name>",
				Description: withExamples("Secret bytes are read from --file or stdin; command-line values are never accepted.",
					"remotr secret upload repositories/private --file /run/secrets/repository-token --fleet production",
					"remotr secret upload services/api --file - --endpoint endpoint-1"),
				Action: actionSecretUpload,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "protected input file, or - for stdin"},
					&cli.BoolFlag{Name: "global", Usage: "explicit server-wide scope (mutually exclusive with tighter scopes)"},
					&cli.StringFlag{Name: "fleet", Usage: "Fleet scope (mutually exclusive with --global and --endpoint)"},
					&cli.StringFlag{Name: "endpoint", Usage: "endpoint scope (mutually exclusive with --global and --fleet)"},
				),
			},
			{Name: "list", Usage: "list authorized logical secrets", Action: actionSecretList, Flags: outputFlags()},
			{Name: "show", Usage: "show safe version metadata for one logical secret", ArgsUsage: "[logical-name]", Action: actionSecretShow, Flags: outputFlags()},
			{Name: "activate", Usage: "activate a version through audited rollout planning", ArgsUsage: "<logical-name> <version>", Action: actionSecretActivate, Flags: outputFlags()},
			{Name: "revoke", Usage: "block future resolution of a version", ArgsUsage: "<logical-name> <version>", Action: actionSecretRevoke, Flags: append(outputFlags(), confirmFlag("logical-name@version"))},
		},
	}
}

func actionSecretUpload(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return exitErr(2, "secret upload requires exactly one secret name; material must come from --file or stdin")
	}
	fleet, endpointID := strings.TrimSpace(c.String("fleet")), strings.TrimSpace(c.String("endpoint"))
	global := c.Bool("global")
	selectedScopes := 0
	if global {
		selectedScopes++
	}
	if fleet != "" {
		selectedScopes++
	}
	if endpointID != "" {
		selectedScopes++
	}
	if selectedScopes != 1 {
		return exitErr(2, "secret upload requires exactly one of --global, --fleet, or --endpoint")
	}
	scope := secrets.ScopeGlobal
	if fleet != "" {
		scope = secrets.ScopeFleet
	} else if endpointID != "" {
		scope = secrets.ScopeEndpoint
	}
	material, err := readProtectedSecretInput(strings.TrimSpace(c.String("file")), os.Stdin, uint32(os.Getuid()))
	if err != nil {
		return exitErr(2, "secret upload: %v", err)
	}
	defer clear(material)
	client, err := secretAdminClient(c, "secret upload")
	if err != nil {
		return err
	}
	metadata, err := client.UploadSecretVersionScoped(strings.TrimSpace(args[0]), scope, fleet, endpointID, material)
	if err != nil {
		return apiErr(c, "secret upload", err)
	}
	return printSecretMetadata(c, metadata, "uploaded inactive secret version")
}

func actionSecretList(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 0 {
		return exitErr(2, "secret list no longer accepts a secret name; use `remotr secret show %s`", strings.TrimSpace(args[0]))
	}
	client, err := secretAdminClient(c, "secret list")
	if err != nil {
		return err
	}
	items := []admin.LogicalSecretSummary{}
	cursor := ""
	for pages := 0; pages < 100; pages++ {
		page, err := client.ListLogicalSecrets(context.Background(), cursor, 100)
		if err != nil {
			return apiErr(c, "secret list", err)
		}
		items = append(items, page.Items...)
		if page.NextCursor == "" {
			if resolveFormat(c) == formatJSON {
				return encodeJSON(items)
			}
			for _, item := range items {
				active := item.ActiveVersion
				if active == "" {
					active = "-"
				}
				fmt.Printf("%s scope=%s active=%s versions=%d\n", item.Name, item.Scope, active, item.VersionCount)
			}
			return nil
		}
		if page.NextCursor == cursor {
			return exitErr(1, "secret list returned a non-advancing cursor")
		}
		cursor = page.NextCursor
	}
	return exitErr(1, "secret list exceeded the pagination bound")
}

func actionSecretShow(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) > 1 || (len(args) == 1 && strings.TrimSpace(args[0]) == "") {
		return exitErr(2, "secret show accepts at most one secret ID")
	}
	secretID := ""
	if len(args) == 1 {
		secretID = strings.TrimSpace(args[0])
	}
	if secretID == "" && (resolveFormat(c) == formatJSON || !isInteractive()) {
		return exitErr(2, "secret show requires a secret ID in non-interactive or structured output; run `remotr secret list` first")
	}
	client, err := secretAdminClient(c, "secret show")
	if err != nil {
		return err
	}
	if secretID == "" {
		items := []admin.LogicalSecretSummary{}
		cursor := ""
		for pages := 0; pages < 100; pages++ {
			page, err := client.ListLogicalSecrets(context.Background(), cursor, 100)
			if err != nil {
				return apiErr(c, "secret show", err)
			}
			items = append(items, page.Items...)
			if page.NextCursor == "" {
				break
			}
			if page.NextCursor == cursor {
				return exitErr(1, "secret show picker inventory returned a non-advancing cursor")
			}
			cursor = page.NextCursor
		}
		if len(items) == 0 {
			return exitErr(1, "no secrets available")
		}
		if err := promptSecretSelect(&secretID, items); err != nil {
			return exitErr(2, "secret show selection cancelled: %v", err)
		}
		secretID = strings.TrimSpace(secretID)
		if secretID == "" {
			return exitErr(2, "secret show selection cancelled")
		}
	}
	metadata, err := client.ListSecretVersions(secretID)
	if err != nil {
		return apiErr(c, "secret show (selection may be stale; run `remotr secret list` and retry)", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(metadata)
	}
	for _, version := range metadata {
		fmt.Printf("%s@%s active=%t revoked=%t fingerprint=%s\n", version.Name, version.Version, version.Active, version.Revoked, version.Fingerprint)
	}
	return nil
}

func actionSecretActivate(_ context.Context, c *cli.Command) error {
	return actionSecretLifecycle(c, "activate")
}

func actionSecretRevoke(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) == 2 {
		if err := requireConfirm(c, "secret revoke", strings.TrimSpace(args[0])+"@"+strings.TrimSpace(args[1])); err != nil {
			return err
		}
	}
	return actionSecretLifecycle(c, "revoke")
}

func actionSecretLifecycle(c *cli.Command, action string) error {
	args := c.Args().Slice()
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		return exitErr(2, "secret %s requires a logical name and exact version", action)
	}
	client, err := secretAdminClient(c, "secret "+action)
	if err != nil {
		return err
	}
	var metadata admin.SecretVersionMetadata
	if action == "activate" {
		metadata, err = client.ActivateSecretVersion(strings.TrimSpace(args[0]), strings.TrimSpace(args[1]))
	} else {
		metadata, err = client.RevokeSecretVersion(strings.TrimSpace(args[0]), strings.TrimSpace(args[1]))
	}
	if err != nil {
		return apiErr(c, "secret "+action, err)
	}
	return printSecretMetadata(c, metadata, action+"d secret version")
}

func secretAdminClient(c *cli.Command, operation string) (*admin.Client, error) {
	settings, err := resolveSettings(c)
	if err != nil {
		return nil, exitErr(2, "%s: %v", operation, err)
	}
	if err := requireOperatorCLI(settings, operation); err != nil {
		return nil, err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return nil, exitErr(1, "%s: %v", operation, err)
	}
	return client, nil
}

func printSecretMetadata(c *cli.Command, metadata admin.SecretVersionMetadata, verb string) error {
	if resolveFormat(c) == formatJSON {
		return encodeJSON(metadata)
	}
	fmt.Printf("%s %s@%s\n", verb, metadata.Name, metadata.Version)
	fmt.Printf("fingerprint: %s\n", metadata.Fingerprint)
	fmt.Printf("scope: %s\n", metadata.Scope)
	fmt.Printf("active: %t\n", metadata.Active)
	fmt.Printf("revoked: %t\n", metadata.Revoked)
	if metadata.EndpointCopyStatus != "" {
		fmt.Printf("endpoint copies: %s\n", metadata.EndpointCopyStatus)
	}
	if len(metadata.Rollouts) > 0 {
		fleets := make(map[string]struct{})
		for _, rollout := range metadata.Rollouts {
			fleets[rollout.Fleet] = struct{}{}
		}
		fmt.Printf("rollout impact: %d resources across %d fleets\n", len(metadata.Rollouts), len(fleets))
	}
	return nil
}

func readProtectedSecretInput(path string, stdin io.Reader, requiredUID uint32) ([]byte, error) {
	if path == "" || path == "-" {
		material, err := io.ReadAll(io.LimitReader(stdin, secrets.MaxMaterialBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		if len(material) == 0 || len(material) > secrets.MaxMaterialBytes {
			clear(material)
			return nil, fmt.Errorf("stdin material is empty or exceeds %d bytes", secrets.MaxMaterialBytes)
		}
		return material, nil
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open protected input file: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open protected input file: invalid descriptor")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, fmt.Errorf("inspect protected input file: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != requiredUID {
		return nil, fmt.Errorf("protected input must be a regular file owned by uid %d", requiredUID)
	}
	if stat.Mode&0o077 != 0 {
		return nil, fmt.Errorf("protected input file must use mode 0600 or stricter")
	}
	material, err := io.ReadAll(io.LimitReader(file, secrets.MaxMaterialBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read protected input file: %w", err)
	}
	if len(material) == 0 || len(material) > secrets.MaxMaterialBytes {
		clear(material)
		return nil, fmt.Errorf("protected input is empty or exceeds %d bytes", secrets.MaxMaterialBytes)
	}
	return material, nil
}
