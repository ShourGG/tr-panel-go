package api

import (
	"testing"

	"terraria-panel/config"
)

func TestPluginDownloadURLsAllowOfficialRepositoryMirror(t *testing.T) {
	cfg := &config.Config{
		UseGitHubMirror:          true,
		GitHubMirrorURL:          "https://mirror.example/",
		GitHubMirrorAllowedRepos: []string{"ShourGG/tr-panel-go"},
	}

	wantMirrorPrefix := "https://mirror.example/"
	for name, urls := range map[string][]string{
		"plugin store":   buildPluginStoreURLs(cfg),
		"plugin package": buildPluginZipURLs(cfg),
	} {
		t.Run(name, func(t *testing.T) {
			if len(urls) < 2 {
				t.Fatalf("URLs = %#v, want mirror and original fallback", urls)
			}
			if urls[0] != wantMirrorPrefix+map[string]string{
				"plugin store":   PluginsJSONURLOriginal,
				"plugin package": PluginsZipURLOriginal,
			}[name] {
				t.Fatalf("first URL = %q, want approved mirror URL", urls[0])
			}
			last := urls[len(urls)-1]
			if name == "plugin store" && last != PluginsJSONURLOriginal {
				t.Fatalf("store fallback URL = %q, want original", last)
			}
			if name == "plugin package" && last != PluginsZipURLOriginal {
				t.Fatalf("package fallback URL = %q, want original", last)
			}
		})
	}
}
