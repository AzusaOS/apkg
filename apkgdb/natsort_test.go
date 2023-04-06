package apkgdb

import "testing"

func TestNatSort(t *testing.T) {
	list := []string{
		"media-video.ffmpeg.libs.5.1.3.linux.amd64",
		"media-video.ffmpeg.libs.5.0.1.linux.amd64",
	}
	natSort(list)

	if list[0] != "media-video.ffmpeg.libs.5.0.1.linux.amd64" {
		t.Errorf("bad natsort result = %v", list)
	}
}
