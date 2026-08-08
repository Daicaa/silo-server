package playback

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReportSegmentDownloadedPrunesOnlyExpiredBackBuffer(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-time.Minute)
	for segment := 0; segment <= 15; segment++ {
		writePrunerTestFile(t, filepath.Join(dir, segmentFilename(segment, TranscodeOpts{})), []byte("segment"), old)
	}
	writePrunerTestFile(t, filepath.Join(dir, "init.mp4"), []byte("init"), old)
	writePrunerTestFile(t, filepath.Join(dir, "stream.m3u8"), []byte("manifest"), old)
	writePrunerTestFile(t, filepath.Join(dir, "seg_00004.ts.tmp"), []byte("temporary"), old)
	writePrunerTestFile(t, filepath.Join(dir, "notes.txt"), []byte("other"), old)
	// A newly-created file below the floor may belong to a replacement ffmpeg
	// process and must survive this pass.
	writePrunerTestFile(t, filepath.Join(dir, segmentFilename(4, TranscodeOpts{})), []byte("fresh"), time.Now())

	session := &TranscodeSession{
		opts: TranscodeOpts{
			SessionID:               "prune-test",
			SegmentDuration:         2,
			SegmentRetentionSeconds: 10,
		},
		outputDir:            dir,
		lastPruneFloor:       -1,
		lastRequestedSegment: 0,
	}
	session.ReportSegmentDownloaded(15)
	waitForPrunePass(t, session)

	for _, segment := range []int{0, 1, 2, 4, 10, 11, 12, 13, 14, 15} {
		assertPrunerFileExists(t, filepath.Join(dir, segmentFilename(segment, TranscodeOpts{})))
	}
	for _, segment := range []int{3, 5, 6, 7, 8, 9} {
		assertPrunerFileMissing(t, filepath.Join(dir, segmentFilename(segment, TranscodeOpts{})))
	}
	for _, name := range []string{"init.mp4", "stream.m3u8", "seg_00004.ts.tmp", "notes.txt"} {
		assertPrunerFileExists(t, filepath.Join(dir, name))
	}
}

func TestReportSegmentDownloadedDisabledKeepsSegments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, segmentFilename(3, TranscodeOpts{}))
	writePrunerTestFile(t, path, []byte("segment"), time.Now().Add(-time.Minute))
	session := &TranscodeSession{
		opts:                 TranscodeOpts{SegmentDuration: 2},
		outputDir:            dir,
		lastPruneFloor:       -1,
		lastRequestedSegment: 0,
	}

	session.ReportSegmentDownloaded(500)
	time.Sleep(20 * time.Millisecond)
	assertPrunerFileExists(t, path)
}

func TestOldGenerationDownloadDoesNotAdvanceRestartedSession(t *testing.T) {
	session := &TranscodeSession{
		opts:                 TranscodeOpts{SegmentRetentionSeconds: 600},
		lastRequestedSegment: 100,
	}
	oldGeneration := session.SegmentGeneration()

	session.mu.Lock()
	session.segmentGeneration++
	session.lastRequestedSegment = 20
	session.mu.Unlock()

	session.ReportSegmentDownloadedForGeneration(101, oldGeneration)
	if got := session.LastRequestedSegment(); got != 20 {
		t.Fatalf("last requested segment = %d, want restarted position 20", got)
	}
}

func TestOpenSegmentDescriptorSurvivesUnlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seg_00001.ts")
	want := []byte("complete segment")
	writePrunerTestFile(t, path, want, time.Now())
	session := &TranscodeSession{outputDir: dir}

	segment, _, err := session.OpenSegment("seg_00001.ts")
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}
	defer func() { _ = segment.Close() }()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove opened segment: %v", err)
	}
	got, err := io.ReadAll(segment)
	if err != nil {
		t.Fatalf("read opened segment: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("opened segment = %q, want %q", got, want)
	}
}

func writePrunerTestFile(t *testing.T, path string, contents []byte, modTime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set times for %s: %v", filepath.Base(path), err)
	}
}

func waitForPrunePass(t *testing.T, session *TranscodeSession) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session.mu.Lock()
		running := session.segmentPruneRunning
		floor := session.lastPruneFloor
		session.mu.Unlock()
		if !running && floor >= 10 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("segment prune pass did not finish")
}

func assertPrunerFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", filepath.Base(path), err)
	}
}

func assertPrunerFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat error = %v", filepath.Base(path), err)
	}
}
