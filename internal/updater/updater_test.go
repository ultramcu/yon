package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		cur, rem string
		want     bool
	}{
		{"0.3.0", "0.4.0", true},
		{"0.3.0", "0.3.1", true},
		{"0.3.0", "1.0.0", true},
		{"0.3.0", "0.3.0", false},
		{"0.3.0", "0.2.9", false},
		{"1.0.0", "0.9.9", false},
		{"v0.3.0", "v0.4.0", true},   // leading v tolerated on both sides
		{"0.3.0", "0.4.0-rc1", true}, // pre-release suffix ignored
		{"0.3", "0.3.1", true},       // short current
		{"", "0.4.0", false},         // unknown current -> not newer
		{"0.3.0", "garbage", false},  // unparsable remote -> not newer
	}
	for _, c := range cases {
		if got := IsNewer(c.cur, c.rem); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.cur, c.rem, got, c.want)
		}
	}
}

func sampleRelease() Release {
	return Release{
		Version: "0.4.0",
		Assets: []Asset{
			{Name: "Yon-v0.4.0-macos-universal.zip"},
			{Name: "Yon-v0.4.0-windows.zip"},
			{Name: "Yon-v0.4.0-linux.tar.xz"},
		},
	}
}

func TestAssetFor(t *testing.T) {
	r := sampleRelease()
	cases := map[string]string{
		"darwin":  "Yon-v0.4.0-macos-universal.zip",
		"windows": "Yon-v0.4.0-windows.zip",
		"linux":   "Yon-v0.4.0-linux.tar.xz",
	}
	for goos, want := range cases {
		a, ok := r.AssetFor(goos)
		if !ok || a.Name != want {
			t.Errorf("AssetFor(%q) = %q (ok=%v), want %q", goos, a.Name, ok, want)
		}
	}
	if _, ok := r.AssetFor("plan9"); ok {
		t.Error("AssetFor(plan9) should not match")
	}
}

func TestLatest(t *testing.T) {
	const body = `{
		"tag_name": "v0.4.0",
		"html_url": "https://github.com/ultramcu/yon/releases/tag/v0.4.0",
		"assets": [
			{"name": "Yon-v0.4.0-macos-universal.zip", "browser_download_url": "https://example.com/mac.zip", "size": 42},
			{"name": "Yon-v0.4.0-linux.tar.xz", "browser_download_url": "https://example.com/linux.txz", "size": 7}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("Latest must send a User-Agent (GitHub rejects requests without one)")
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	old := apiURL
	apiURL = srv.URL
	defer func() { apiURL = old }()

	rel, err := Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "0.4.0" || rel.TagName != "v0.4.0" {
		t.Errorf("version = %q / tag = %q, want 0.4.0 / v0.4.0", rel.Version, rel.TagName)
	}
	if len(rel.Assets) != 2 || rel.Assets[0].URL != "https://example.com/mac.zip" || rel.Assets[0].Size != 42 {
		t.Errorf("assets parsed wrong: %+v", rel.Assets)
	}
}

func TestLatest_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	old := apiURL
	apiURL = srv.URL
	defer func() { apiURL = old }()

	if _, err := Latest(context.Background()); err == nil {
		t.Error("Latest should error on a non-200 response")
	}
}

func TestDownload(t *testing.T) {
	const payload = "binary-bytes-here"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := Download(context.Background(), Asset{Name: "Yon-v0.4.0-linux.tar.xz", URL: srv.URL}, dir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if path != filepath.Join(dir, "Yon-v0.4.0-linux.tar.xz") {
		t.Errorf("saved to %q, want under %q", path, dir)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != payload {
		t.Errorf("downloaded content = %q (err %v), want %q", got, err, payload)
	}
}
