package jellycompat

import (
	"testing"

	"github.com/Silo-Server/silo-server/internal/config"
)

func TestPlaybackHandlerReadsLiveSegmentRetention(t *testing.T) {
	cfg := &config.Config{Playback: config.PlaybackConfig{SegmentRetentionSeconds: 600}}
	handler := NewPlaybackHandler(cfg, nil, nil, nil, nil, nil, nil, nil)
	liveRetention := 600
	handler.SegmentRetentionSeconds = func() int { return liveRetention }

	if got := handler.tm.Config().SegmentRetentionSeconds; got != 600 {
		t.Fatalf("initial segment retention = %d, want 600", got)
	}
	liveRetention = 120
	if got := handler.tm.Config().SegmentRetentionSeconds; got != 120 {
		t.Fatalf("reloaded segment retention = %d, want 120", got)
	}
}
