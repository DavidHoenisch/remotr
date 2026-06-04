package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DemoEnabled reports whether the operator CLI should serve admin API responses
// from static fixtures instead of the network (REMOTR_DEMO=1 or true).
func DemoEnabled() bool {
	v := strings.TrimSpace(os.Getenv("REMOTR_DEMO"))
	return v == "1" || strings.EqualFold(v, "true")
}

func demoFixturesDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("REMOTR_DEMO_FIXTURES")); v != "" {
		return filepath.Clean(v), nil
	}
	return "", fmt.Errorf("REMOTR_DEMO is set but REMOTR_DEMO_FIXTURES is empty")
}

func demoHTTPClient() (*http.Client, error) {
	dir, err := demoFixturesDir()
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: &demoTransport{dir: dir}}, nil
}

type demoTransport struct {
	dir string
}

type demoFixture struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func (t *demoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key, err := fixtureKey(req.Method, req.URL.Path)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(filepath.Join(t.dir, key+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return jsonResponse(http.StatusNotFound, map[string]string{
				"error": fmt.Sprintf("demo fixture not found: %s", key),
			}), nil
		}
		return nil, err
	}

	var fix demoFixture
	if err := json.Unmarshal(raw, &fix); err != nil {
		return nil, fmt.Errorf("parse demo fixture %s: %w", key, err)
	}
	if fix.Status == 0 {
		fix.Status = http.StatusOK
	}

	var body []byte
	if len(fix.Body) > 0 && string(fix.Body) != "null" {
		body = fix.Body
	}
	return fixtureHTTPResponse(fix.Status, fix.Headers, body), nil
}

func fixtureKey(method, path string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	if method == "" || path == "" {
		return "", fmt.Errorf("demo fixture: empty method or path")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, seg := range segments {
		segments[i] = strings.ReplaceAll(seg, ".", "_")
	}
	return method + "_" + strings.Join(segments, "_"), nil
}

func fixtureHTTPResponse(status int, headers map[string]string, body []byte) *http.Response {
	if headers == nil {
		headers = map[string]string{}
	}
	if _, ok := headers["Content-Type"]; !ok && len(body) > 0 {
		headers["Content-Type"] = "application/json"
	}
	hdr := make(http.Header)
	for k, v := range headers {
		hdr.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     hdr,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func jsonResponse(status int, v any) *http.Response {
	body, _ := json.Marshal(v)
	return fixtureHTTPResponse(status, nil, body)
}
