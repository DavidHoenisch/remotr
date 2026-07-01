package pwa

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	managedComment   = "Managed by Remotr"
	desktopPrefix    = "remotr-pwa-"
	usersInteractive = "interactive"
)

var defaultBrowsers = []string{
	"chromium",
	"google-chrome-stable",
	"google-chrome",
	"brave-browser",
	"microsoft-edge-stable",
	"microsoft-edge",
}

var slugSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// Applicator installs per-user PWA launcher desktop entries.
type Applicator struct {
	Package   models.Package
	Exec      executil.Runner
	ListUsers func() ([]interactiveuser.Account, error)
	FetchURL  func(ctx context.Context, rawURL string) ([]byte, error)
}

func New(pkg models.Package, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Package: pkg, Exec: exec}
}

func (a *Applicator) Name() string {
	return "pwa:" + a.slug() + ":" + strings.TrimSpace(a.Package.PWAURL)
}

func (a *Applicator) Description() string {
	title := a.title()
	return fmt.Sprintf("pwa %s (%s)", title, strings.TrimSpace(a.Package.PWAURL))
}

func (a *Applicator) listUsers() ([]interactiveuser.Account, error) {
	fn := a.ListUsers
	if fn == nil {
		fn = interactiveuser.List
	}
	return fn()
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	users, err := a.listUsers()
	if err != nil {
		return nil, false
	}
	if len(users) == 0 {
		return nil, !a.Package.Present
	}
	for _, u := range users {
		ok, err := a.userState(u)
		if err != nil || !ok {
			return nil, false
		}
	}
	return nil, true
}

func (a *Applicator) Apply(ctx context.Context) error {
	users, err := a.listUsers()
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return fmt.Errorf("no interactive users found")
	}
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}
	browser, err := a.resolveBrowser()
	if err != nil {
		return err
	}
	anyApplied := false
	for _, u := range users {
		ok, err := a.userState(u)
		if err != nil {
			return fmt.Errorf("user %s: %w", u.Username, err)
		}
		if ok {
			continue
		}
		if err := a.applyUser(ctx, u, browser); err != nil {
			return fmt.Errorf("user %s: %w", u.Username, err)
		}
		anyApplied = true
	}
	if !anyApplied {
		return appErr.ErrStateAlreadyMet
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) userState(u interactiveuser.Account) (bool, error) {
	desktopPath, err := a.desktopPath(u.HomeDir)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(desktopPath)
	if a.Package.Present {
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		if info.IsDir() {
			return false, nil
		}
		data, err := os.ReadFile(desktopPath) // #nosec G304 -- path under validated home
		if err != nil {
			return false, err
		}
		return desktopMatches(string(data), strings.TrimSpace(a.Package.PWAURL)), nil
	}
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (a *Applicator) applyUser(ctx context.Context, u interactiveuser.Account, browser string) error {
	desktopPath, err := a.desktopPath(u.HomeDir)
	if err != nil {
		return err
	}
	if !a.Package.Present {
		if err := os.Remove(desktopPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		iconPath, err := a.iconPath(u.HomeDir)
		if err != nil {
			return err
		}
		_ = os.Remove(iconPath)
		return nil
	}
	desktopDir := filepath.Dir(desktopPath)
	if err := ensureUserTreeDir(u.HomeDir, desktopDir, u.UID, u.GID); err != nil {
		return err
	}
	if iconURL := strings.TrimSpace(a.Package.PWAIcon); iconURL != "" {
		iconPath, err := a.iconPath(u.HomeDir)
		if err != nil {
			return err
		}
		if err := ensureUserTreeDir(u.HomeDir, filepath.Dir(iconPath), u.UID, u.GID); err != nil {
			return err
		}
		if err := a.ensureIcon(ctx, iconPath, iconURL); err != nil {
			return err
		}
		if err := os.Chown(iconPath, u.UID, u.GID); err != nil {
			return err
		}
	}
	content, err := a.desktopContent(u.HomeDir, browser)
	if err != nil {
		return err
	}
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil { // #nosec G306 G703
		return err
	}
	return os.Chown(desktopPath, u.UID, u.GID)
}

func (a *Applicator) resolveBrowser() (string, error) {
	if b := strings.TrimSpace(a.Package.PWABrowser); b != "" {
		stdout, _, err := a.Exec.Run("which", b)
		if err != nil {
			return "", fmt.Errorf("pwaBrowser %q not found in PATH", b)
		}
		return strings.TrimSpace(string(stdout)), nil
	}
	for _, name := range defaultBrowsers {
		stdout, _, err := a.Exec.Run("which", name)
		if err == nil {
			return strings.TrimSpace(string(stdout)), nil
		}
	}
	return "", fmt.Errorf("no supported browser found; set pwaBrowser")
}

func (a *Applicator) desktopContent(homeDir, browser string) (string, error) {
	title := a.title()
	appURL := strings.TrimSpace(a.Package.PWAURL)
	wmClass, err := startupWMClass(appURL)
	if err != nil {
		return "", err
	}
	iconName := a.iconName()
	execLine := fmt.Sprintf("%s --app=%s", browser, appURL)
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Version=1.0\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=" + title + "\n")
	b.WriteString("Comment=" + managedComment + "\n")
	b.WriteString("Exec=" + execLine + "\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("StartupNotify=true\n")
	b.WriteString("StartupWMClass=" + wmClass + "\n")
	if iconName != "" {
		b.WriteString("Icon=" + iconName + "\n")
	}
	b.WriteString("Categories=Network;\n")
	return b.String(), nil
}

func desktopMatches(got, appURL string) bool {
	if !strings.Contains(got, managedComment) {
		return false
	}
	if !strings.Contains(got, appURL) {
		return false
	}
	wantWM, err := startupWMClass(appURL)
	if err != nil {
		return false
	}
	return strings.Contains(got, "StartupWMClass="+wantWM)
}

func (a *Applicator) desktopPath(homeDir string) (string, error) {
	return interactiveuser.HomePath(homeDir, filepath.Join(".local", "share", "applications", desktopPrefix+a.slug()+".desktop"))
}

func (a *Applicator) iconPath(homeDir string) (string, error) {
	return interactiveuser.HomePath(homeDir, filepath.Join(".local", "share", "icons", "hicolor", "256x256", "apps", a.iconName()+".png"))
}

func (a *Applicator) iconName() string {
	return desktopPrefix + a.slug()
}

func (a *Applicator) slug() string {
	s := slugSanitizer.ReplaceAllString(strings.TrimSpace(a.Package.Name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "app"
	}
	return s
}

func (a *Applicator) title() string {
	if t := strings.TrimSpace(a.Package.PWATitle); t != "" {
		return t
	}
	return strings.TrimSpace(a.Package.Name)
}

func startupWMClass(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid pwaURL: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("pwaURL must include a host")
	}
	return u.Host, nil
}

func (a *Applicator) ensureIcon(ctx context.Context, dest, rawURL string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := a.fetch(ctx, rawURL)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644) // #nosec G306 G703
}

func localShareAnchor(homeDir string) string {
	return filepath.Join(homeDir, ".local", "share")
}

// ensureUserTreeDir creates dir (and parents) and assigns uid/gid to each directory
// from the first segment under ~/.local/share through dir. The agent runs as root;
// without chowning intermediate directories, interactive users cannot traverse them.
func ensureUserTreeDir(homeDir, dir string, uid, gid int) error {
	dir = filepath.Clean(dir)
	anchor := localShareAnchor(homeDir)
	if dir != anchor && !strings.HasPrefix(dir, anchor+string(os.PathSeparator)) {
		return fmt.Errorf("directory %q is outside %q", dir, anchor)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return chownDirChain(dir, anchor, uid, gid)
}

func chownDirChain(dir, anchor string, uid, gid int) error {
	dir = filepath.Clean(dir)
	anchor = filepath.Clean(anchor)
	for cur := dir; cur != anchor; cur = filepath.Dir(cur) {
		if err := os.Chown(cur, uid, gid); err != nil {
			return err
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return nil
}

func (a *Applicator) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	if err := validateFetchURL(rawURL); err != nil {
		return nil, err
	}
	fn := a.FetchURL
	if fn != nil {
		return fn(ctx, rawURL)
	}
	out, _, err := a.Exec.Run("curl", "-fsSL", rawURL)
	if err == nil {
		return out, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func validateFetchURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid URL %q", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL %q must use http or https", rawURL)
	}
	return nil
}
