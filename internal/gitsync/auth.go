package gitsync

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

func (g *GitSyncer) gitUsername() string {
	if strings.TrimSpace(g.Username) != "" {
		return strings.TrimSpace(g.Username)
	}
	return "x-access-token"
}

func (g *GitSyncer) authConfigArgs() []string {
	token := strings.TrimSpace(g.Token)
	if token == "" {
		return nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte(g.gitUsername() + ":" + token))
	header := "Authorization: Basic " + auth
	if host := gitHost(g.RemoteURL); host != "" {
		key := fmt.Sprintf("http.https://%s/.extraHeader", host)
		return []string{"-c", key + "=" + header}
	}
	return []string{"-c", "http.extraHeader=" + header}
}

func gitHost(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func cleanRemoteURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}
