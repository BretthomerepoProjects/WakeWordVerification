//go:build spot && wakewordverification

package update

import "testing"

func TestProjectSpotFollowsOneManualReleaseFeed(t *testing.T) {
	const want = "https://github.com/BretthomerepoProjects/WakeWordVerification/releases/latest/download/manifest.json"
	if Stable.URL() != want || Dev.URL() != want {
		t.Fatalf("unexpected project URLs: stable %q, dev %q", Stable.URL(), Dev.URL())
	}
	if channels := Channels(); len(channels) != 1 || channels[0] != Stable {
		t.Fatalf("unexpected project channels: %v", channels)
	}
	if !ManualOnly {
		t.Fatal("project Spot builds must require manual installs")
	}
}
