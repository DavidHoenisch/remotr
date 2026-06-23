package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

type errorCode string

const (
	codeCredentialsMissing errorCode = "E_CREDENTIALS_MISSING"
	codeServerURLMissing   errorCode = "E_SERVER_URL_MISSING"
	codeCAMissing          errorCode = "E_CA_MISSING"
	codeFleetMissing       errorCode = "E_FLEET_MISSING"
	codeEndpointMissing    errorCode = "E_ENDPOINT_MISSING"
	codeConfirmRequired    errorCode = "E_CONFIRM_REQUIRED"
	codeDrift              errorCode = "E_DRIFT"
	codeAPI                errorCode = "E_API"
)

type cliError struct {
	Code     errorCode
	Title    string
	Cause    string
	Fix      string
	Hint     string
	Wrapped  error
	exitCode int
}

func (e *cliError) Error() string {
	return e.format(false)
}

func (e *cliError) ExitCode() int {
	return e.exitCode
}

func (e *cliError) format(verbose bool) string {
	var b strings.Builder
	b.WriteString("error: ")
	b.WriteString(e.Title)
	if e.Code != "" {
		b.WriteString("\n  code: ")
		b.WriteString(string(e.Code))
	}
	if e.Cause != "" {
		b.WriteString("\n  cause: ")
		b.WriteString(e.Cause)
	}
	if e.Fix != "" {
		b.WriteString("\n  fix:   ")
		b.WriteString(e.Fix)
	}
	if e.Hint != "" {
		b.WriteString("\n  hint:  ")
		b.WriteString(e.Hint)
	}
	if verbose && e.Wrapped != nil {
		b.WriteString("\n  detail: ")
		b.WriteString(e.Wrapped.Error())
	}
	return b.String()
}

func structuredErr(e *cliError) error {
	return e
}

func printCLIError(c *cli.Command, err error) {
	if e, ok := err.(*cliError); ok {
		writeStyledError(os.Stderr, c, e)
		return
	}
	if ec, ok := err.(cli.ExitCoder); ok {
		if msg := ec.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		}
	}
}

func writeStyledError(w io.Writer, c *cli.Command, e *cliError) {
	fmt.Fprintln(w, labelError(c, "error:")+" "+e.Title)
	if e.Code != "" {
		fmt.Fprintf(w, "  code: %s\n", e.Code)
	}
	if e.Cause != "" {
		fmt.Fprintf(w, "  cause: %s\n", e.Cause)
	}
	if e.Fix != "" {
		fmt.Fprintf(w, "  fix:   %s\n", e.Fix)
	}
	if e.Hint != "" {
		fmt.Fprintf(w, "  hint:  %s\n", e.Hint)
	}
	if c.Bool("verbose") && e.Wrapped != nil {
		fmt.Fprintf(w, "  detail: %v\n", e.Wrapped)
	}
}

func errCredentialsMissing(cmd, stateDir string) error {
	return structuredErr(&cliError{
		Code:     codeCredentialsMissing,
		Title:    "operator credentials missing",
		Cause:    fmt.Sprintf("no operator credentials in %s", stateDir),
		Fix:      "remotr bootstrap --token <token> --server-url <url> --ca <ca.pem>",
		Hint:     "run remotr doctor to diagnose setup",
		exitCode: 2,
	})
}

func errServerURLMissing(cmd string) error {
	return structuredErr(&cliError{
		Code:     codeServerURLMissing,
		Title:    "server URL is required",
		Cause:    "REMOTR_SERVER_URL, config file, and --server-url are all unset",
		Fix:      "remotr config init --server-url https://remotr.example:8443",
		Hint:     "or export REMOTR_SERVER_URL",
		exitCode: 2,
	})
}

func errCAMissing(cmd string) error {
	return structuredErr(&cliError{
		Code:     codeCAMissing,
		Title:    "CA certificate path is required",
		Cause:    "bootstrap needs the Remotr CA PEM to verify the server",
		Fix:      "remotr bootstrap --ca /path/to/ca.crt --token <token> --server-url <url>",
		Hint:     "download from https://<server>/v1/ca.pem",
		exitCode: 2,
	})
}

func errFleetMissing(cmd string) error {
	return structuredErr(&cliError{
		Code:     codeFleetMissing,
		Title:    "fleet is required",
		Cause:    "REMOTR_FLEET, config file, --fleet, and positional argument are all unset",
		Fix:      "remotr fleet cron report --fleet engineering",
		Hint:     "list fleets with remotr fleet list; run in a terminal to pick a fleet interactively",
		exitCode: 2,
	})
}

func errEndpointMissing(cmd string) error {
	return structuredErr(&cliError{
		Code:     codeEndpointMissing,
		Title:    "endpoint id is required",
		Cause:    "--endpoint and positional argument are both unset",
		Fix:      "remotr endpoint show <endpoint-id>",
		Hint:     "list endpoints with remotr endpoint list; run in a terminal to enter an id interactively",
		exitCode: 2,
	})
}

func errConfirmRequired(cmd, resourceID string) error {
	return structuredErr(&cliError{
		Code:     codeConfirmRequired,
		Title:    "confirmation required",
		Cause:    fmt.Sprintf("destructive action on %q", resourceID),
		Fix:      fmt.Sprintf("%s --confirm %s", cmd, resourceID),
		Hint:     "run interactively in a terminal to confirm when prompted",
		exitCode: 2,
	})
}

func errDrift() error {
	return structuredErr(&cliError{
		Code:     codeDrift,
		Title:    "compliance drift detected",
		Cause:    "one or more endpoints are out of compliance",
		Fix:      "inspect report output and update fleet desired state in Git",
		exitCode: 4,
	})
}

func errAPI(cmd string, wrapped error, verbose bool) error {
	e := &cliError{
		Code:     codeAPI,
		Title:    cmd + " failed",
		Cause:    wrapped.Error(),
		Hint:     "run with --verbose for full details",
		Wrapped:  wrapped,
		exitCode: 1,
	}
	if verbose {
		return structuredErr(e)
	}
	// Shorter cause for non-verbose: first line only
	if idx := strings.Index(e.Cause, "\n"); idx > 0 {
		e.Cause = e.Cause[:idx]
	}
	return structuredErr(e)
}
