package api

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildSteamWorkshopQueryURLUsesConfiguredKeyAndEscapesQuery(t *testing.T) {
	got, err := buildSteamWorkshopQueryURL("test-key", "12", 2, 5, "Recipe Browser & More")
	if err != nil {
		t.Fatalf("build Steam Workshop URL: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse Steam Workshop URL: %v", err)
	}
	values := parsed.Query()
	if values.Get("key") != "test-key" {
		t.Fatalf("Steam API key = %q, want test-key", values.Get("key"))
	}
	if values.Get("query_type") != "12" || values.Get("page") != "2" || values.Get("numperpage") != "5" {
		t.Fatalf("unexpected Steam Workshop pagination parameters: %v", values)
	}
	if values.Get("search_text") != "Recipe Browser & More" {
		t.Fatalf("search_text = %q, want original query", values.Get("search_text"))
	}
}

func TestBuildSteamWorkshopQueryURLRejectsMissingKeyWithoutSensitiveURL(t *testing.T) {
	_, err := buildSteamWorkshopQueryURL("", "12", 1, 5, "Recipe Browser")
	if err == nil {
		t.Fatal("expected missing Steam API key error")
	}
	if strings.Contains(err.Error(), "https://") || strings.Contains(err.Error(), "key=") {
		t.Fatalf("missing-key error exposes a URL or key: %q", err)
	}
}
