package api

import "testing"

func TestSelectLatestChannelReleasePrefersHighestDevVersion(t *testing.T) {
	releases := []githubRelease{
		{TagName: "v1.3.18-dev.9", Prerelease: true, PublishedAt: "2026-04-17T08:00:37Z"},
		{TagName: "v1.3.18-dev.7", Prerelease: true, PublishedAt: "2026-04-17T07:43:37Z"},
		{TagName: "v1.3.18-dev.6", Prerelease: true, PublishedAt: "2026-04-17T07:25:53Z"},
		{TagName: "v1.3.18-dev.10", Prerelease: true, PublishedAt: "2026-04-17T08:37:19Z"},
		{TagName: "v1.3.17", Prerelease: false, PublishedAt: "2026-04-16T15:48:39Z"},
	}

	selected := selectLatestChannelRelease(releases, updateChannelDev)
	if selected == nil {
		t.Fatal("expected a dev release to be selected")
	}

	if selected.TagName != "v1.3.18-dev.10" {
		t.Fatalf("expected dev.10 to be selected, got %s", selected.TagName)
	}
}

func TestSelectLatestChannelReleasePrefersNewestStableVersion(t *testing.T) {
	releases := []githubRelease{
		{TagName: "v1.3.16", Prerelease: false, PublishedAt: "2026-04-16T15:08:30Z"},
		{TagName: "v1.3.17", Prerelease: false, PublishedAt: "2026-04-16T15:48:39Z"},
		{TagName: "v1.3.18-dev.10", Prerelease: true, PublishedAt: "2026-04-17T08:37:19Z"},
	}

	selected := selectLatestChannelRelease(releases, updateChannelStable)
	if selected == nil {
		t.Fatal("expected a stable release to be selected")
	}

	if selected.TagName != "v1.3.17" {
		t.Fatalf("expected stable v1.3.17 to be selected, got %s", selected.TagName)
	}
}

func TestCompareReleasePriorityFallsBackToPublishedAt(t *testing.T) {
	older := githubRelease{TagName: "custom-dev-build", Prerelease: true, PublishedAt: "2026-04-17T08:00:37Z"}
	newer := githubRelease{TagName: "custom-dev-build-hotfix", Prerelease: true, PublishedAt: "2026-04-17T08:37:19Z"}

	if compareReleasePriority(newer, older) <= 0 {
		t.Fatal("expected newer published release to win when tag parsing falls back to publish time")
	}
}
