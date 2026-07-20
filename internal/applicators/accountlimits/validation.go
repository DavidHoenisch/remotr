package accountlimits

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
)

var nativeLimitItems = map[string]struct{}{
	"as": {}, "chroot": {}, "core": {}, "cpu": {}, "data": {}, "fsize": {},
	"locks": {}, "maxlogins": {}, "maxsyslogins": {}, "memlock": {},
	"msgqueue": {}, "nice": {}, "nofile": {}, "nonewprivs": {}, "nproc": {},
	"priority": {}, "rss": {}, "rtprio": {}, "rttime": {}, "sigpending": {},
	"stack": {},
}

// validateConfiguration validates the complete configuration pam_limits will
// read, substituting the desired named fragment in memory before any mutation.
func (a *Applicator) validateConfiguration(candidatePath string) error {
	candidatePath = filepath.Clean(candidatePath)
	mainPath := filepath.Join(filepath.Dir(filepath.Clean(a.LimitsDir)), "limits.conf")
	if content, err := os.ReadFile(mainPath); err == nil {
		if err := validateLimitsContent(content); err != nil {
			return invalidLimitsFile(mainPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read account limits configuration: %w", err)
	}

	entries, err := os.ReadDir(a.LimitsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read account limits directory: %w", err)
	}
	candidateSeen := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		path := filepath.Join(a.LimitsDir, entry.Name())
		if filepath.Clean(path) == candidatePath {
			candidateSeen = true
			if a.Resource.Lifecycle == models.LifecycleAbsent {
				continue
			}
			if err := validateLimitsContent([]byte(a.render())); err != nil {
				return invalidLimitsFile(path, err)
			}
			continue
		}
		content, err := os.ReadFile(path) // #nosec G304 -- bounded native limits.d inventory.
		if err != nil {
			return fmt.Errorf("read account limits configuration: %w", err)
		}
		if err := validateLimitsContent(content); err != nil {
			return invalidLimitsFile(path, err)
		}
	}
	if !candidateSeen && a.Resource.Lifecycle != models.LifecycleAbsent {
		if err := validateLimitsContent([]byte(a.render())); err != nil {
			return invalidLimitsFile(candidatePath, err)
		}
	}
	return nil
}

func invalidLimitsFile(path string, err error) error {
	return fmt.Errorf("invalid account limits configuration %q: %w", filepath.Base(path), err)
}

func validateLimitsContent(content []byte) error {
	if bytes.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("contains a NUL byte")
	}
	for index, raw := range bytes.Split(content, []byte{'\n'}) {
		line := string(raw)
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = line[:comment]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 2 && fields[1] == "-" {
			continue
		}
		if len(fields) != 4 {
			return fmt.Errorf("line %d has invalid field count", index+1)
		}
		if !validNativeLimitDomain(fields[0]) {
			return fmt.Errorf("line %d has invalid limit domain", index+1)
		}
		if fields[1] != "soft" && fields[1] != "hard" && fields[1] != "-" {
			return fmt.Errorf("line %d has invalid limit type", index+1)
		}
		if _, ok := nativeLimitItems[fields[2]]; !ok {
			return fmt.Errorf("line %d has invalid limit item", index+1)
		}
		if !validNativeLimitValue(fields[2], fields[3]) {
			return fmt.Errorf("line %d has invalid limit value", index+1)
		}
	}
	return nil
}

func validNativeLimitDomain(domain string) bool {
	if !strings.ContainsRune(domain, ':') {
		return true
	}
	rangeValue := domain
	if strings.HasPrefix(rangeValue, "@") || strings.HasPrefix(rangeValue, "%") {
		rangeValue = rangeValue[1:]
	}
	if strings.Count(rangeValue, ":") != 1 {
		return false
	}
	bounds := strings.SplitN(rangeValue, ":", 2)
	if bounds[0] == "" && bounds[1] == "" {
		return false
	}
	for _, bound := range bounds {
		if bound == "" {
			continue
		}
		if _, err := strconv.ParseUint(bound, 10, 32); err != nil {
			return false
		}
	}
	return true
}

func validNativeLimitValue(item, value string) bool {
	switch item {
	case "chroot":
		return value != ""
	case "nonewprivs":
		return value == "0" || value == "1"
	}
	if value == "-" || value == "unlimited" || value == "infinity" {
		return true
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
