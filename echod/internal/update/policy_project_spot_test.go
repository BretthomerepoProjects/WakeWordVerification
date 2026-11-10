//go:build spot && wakewordverification

package update

import "testing"

func TestProjectSpotFollowsOneReleaseFeed(t *testing.T) {
	const want = "https://github.com/BretthomerepoProjects/WakeWordVerification/releases/latest/download/manifest.json"
	if Stable.URL() != want || Dev.URL() != want {
		t.Fatalf("unexpected project URLs: stable %q, dev %q", Stable.URL(), Dev.URL())
	}
	if channels := Channels(); len(channels) != 1 || channels[0] != Stable {
		t.Fatalf("unexpected project channels: %v", channels)
	}
	if !ProjectAutoUpdates {
		t.Fatal("project Spot builds must require a project-specific opt-in")
	}
}
