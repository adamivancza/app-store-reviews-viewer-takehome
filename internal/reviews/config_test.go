package reviews

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoadShippedConfig(t *testing.T) {
	app, err := LoadConfig(filepath.Join("..", "..", "config", "app.json"))
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if app.Key != "spotify-us" || app.Name != "Spotify" || app.AppID != "324684580" || app.Country != "us" {
		t.Fatalf("unexpected app identity: %#v", app)
	}
	wantDataDir, err := filepath.Abs(filepath.Join("..", "..", "data"))
	if err != nil {
		t.Fatal(err)
	}
	if app.PollInterval != 5*time.Minute || app.MaxPages != 10 || app.DataDir != wantDataDir || app.ListenAddr != ":8080" {
		t.Fatalf("unexpected runtime config: %#v", app)
	}
}

func TestLoadConfigResolvesDataDirRelativeToConfig(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "app.json")
	contents := `{
      "key":"spotify-us", "name":"Spotify", "appId":"324684580", "country":"us",
      "pollInterval":"5m", "maxPages":10, "dataDir":"../data", "listenAddr":":8080"
    }`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	app, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(configDir), "data")
	if app.DataDir != want || !filepath.IsAbs(app.DataDir) {
		t.Fatalf("DataDir = %q, want absolute %q", app.DataDir, want)
	}
}

func TestLoadConfigRejectsEmptyDataDir(t *testing.T) {
	for _, dataDir := range []string{"", "  \t"} {
		path := filepath.Join(t.TempDir(), "app.json")
		contents := `{
      "key":"spotify-us", "name":"Spotify", "appId":"324684580", "country":"us",
      "pollInterval":"5m", "maxPages":10, "dataDir":` + strconv.Quote(dataDir) + `, "listenAddr":":8080"
    }`
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadConfig(path)
		if err == nil || !strings.Contains(err.Error(), "dataDir is required") {
			t.Fatalf("LoadConfig(%q) returned %v, want dataDir validation error", dataDir, err)
		}
	}
}

func TestLoadConfigRejectsUnsafeAndInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		pages   int
		message string
	}{
		{name: "unsafe key", key: "../spotify", pages: 10, message: "app key"},
		{name: "too many pages", key: "spotify-us", pages: 11, message: "maxPages"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.json")
			contents := `{
              "key": "` + tc.key + `",
              "name": "Spotify",
              "appId": "324684580",
              "country": "us",
              "pollInterval": "5m",
			  "maxPages": ` + strconv.Itoa(tc.pages) + `,
              "dataDir": "data",
              "listenAddr": ":8080"
            }`
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConfig(path)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got error %v, want containing %q", err, tc.message)
			}
		})
	}
}
