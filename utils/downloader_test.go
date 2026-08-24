package utils

import "testing"

func TestBuildDownloadURLsRestrictsMirrorsToAllowedRepository(t *testing.T) {
	allowed := []string{"ShourGG/tr-panel-go"}
	original := "https://github.com/ShourGG/tr-panel-go/releases/download/v1.0.0/tr-panel-linux-amd64"
	urls := BuildDownloadURLs(original, true, "https://mirror.example/", allowed)

	if len(urls) < 2 {
		t.Fatalf("expected mirror and original URLs, got %#v", urls)
	}
	if urls[0] != "https://mirror.example/"+original {
		t.Fatalf("first URL = %q, want configured mirror URL", urls[0])
	}
	if urls[len(urls)-1] != original {
		t.Fatalf("last URL = %q, want original fallback", urls[len(urls)-1])
	}
}

func TestBuildDownloadURLsAllowsApprovedRawGitHubURL(t *testing.T) {
	original := "https://raw.githubusercontent.com/ShourGG/tr-panel-go/main/README.md"
	urls := BuildDownloadURLs(original, true, "https://mirror.example", []string{"shourgg/tr-panel-go"})
	if len(urls) < 2 || urls[0] != "https://mirror.example/"+original {
		t.Fatalf("approved raw GitHub URL should use mirror, got %#v", urls)
	}
}

func TestBuildDownloadURLsRejectsUnapprovedOrSpoofedGitHubURLs(t *testing.T) {
	testCases := []string{
		"https://github.com/Pryaxis/TShock/releases/download/v6/TShock.zip",
		"https://github.com.evil.example/ShourGG/tr-panel-go/releases/download/v1/file",
		"https://evil.example/github.com/ShourGG/tr-panel-go/releases/download/v1/file",
		"https://example.com/download/file.zip",
	}

	for _, original := range testCases {
		t.Run(original, func(t *testing.T) {
			urls := BuildDownloadURLs(original, true, "https://mirror.example/", []string{"ShourGG/tr-panel-go"})
			if len(urls) != 1 || urls[0] != original {
				t.Fatalf("unapproved URL must use official source only, got %#v", urls)
			}
		})
	}
}

func TestBuildDownloadURLsDefaultsToPanelRepositoryAllowlist(t *testing.T) {
	original := "https://github.com/Pryaxis/TShock/releases/download/v6/TShock.zip"
	urls := BuildDownloadURLs(original, true, "https://mirror.example/", nil)
	if len(urls) != 1 || urls[0] != original {
		t.Fatalf("empty allowlist must not open the mirror to arbitrary repositories, got %#v", urls)
	}
}
