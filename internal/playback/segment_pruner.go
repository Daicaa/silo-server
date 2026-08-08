package playback

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const segmentPruneHysteresis = 5

// scheduleSegmentPruneLocked starts one asynchronous prune pass after the
// client has advanced far enough to make useful work. The caller must hold
// s.mu. Segment retention is measured in media time so custom HLS segment
// durations retain the same back-seek window.
func (s *TranscodeSession) scheduleSegmentPruneLocked() {
	retentionSeconds := s.opts.SegmentRetentionSeconds
	if retentionSeconds <= 0 || s.segmentPruneRunning || s.restarting {
		return
	}
	segmentDuration := s.opts.SegmentDuration
	if segmentDuration <= 0 {
		segmentDuration = defaultSegmentDuration
	}
	retainedSegments := (retentionSeconds + segmentDuration - 1) / segmentDuration
	floor := s.lastRequestedSegment - retainedSegments
	if floor <= s.opts.StartSegmentNumber || floor-s.lastPruneFloor < segmentPruneHysteresis {
		return
	}

	s.segmentPruneRunning = true
	generation := s.segmentGeneration
	go s.pruneDownloadedSegments(generation, floor)
}

// pruneDownloadedSegments removes completed media files strictly behind floor.
// FFmpeg's manifest and init files are never candidates, and the current
// process's startup window remains present so real-manifest reloads cannot get
// stuck behind startupFilesReady after cleanup begins.
func (s *TranscodeSession) pruneDownloadedSegments(generation uint64, floor int) {
	started := time.Now()
	entries, err := os.ReadDir(s.outputDir)
	if err != nil {
		s.finishSegmentPrune(generation, floor)
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("read transcode segments for pruning", "component", "playback", "error", err, "session", s.opts.SessionID, "playback_session_id", s.opts.SessionID)
		}
		return
	}

	s.mu.Lock()
	opts := s.opts
	s.mu.Unlock()
	segmentDuration := opts.SegmentDuration
	if segmentDuration <= 0 {
		segmentDuration = defaultSegmentDuration
	}
	freshAfter := time.Now().Add(-(2*time.Duration(segmentDuration)*time.Second + 30*time.Second))
	startupEnd := opts.StartSegmentNumber + startupSegmentRequirement(opts)

	removed := 0
	var freedBytes int64
	for _, entry := range entries {
		segment, parseErr := ParseSegmentNumber(entry.Name())
		if parseErr != nil || segment >= floor || (segment >= opts.StartSegmentNumber && segment < startupEnd) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || info.ModTime().After(freshAfter) {
			continue
		}

		// Serialize the unlink with restart's generation change. Once restart
		// advances the generation, this pass cannot delete newly generated files.
		s.mu.Lock()
		if generation != s.segmentGeneration || s.restarting {
			s.mu.Unlock()
			s.finishSegmentPrune(generation, floor)
			return
		}
		removeErr := os.Remove(filepath.Join(s.outputDir, entry.Name()))
		s.mu.Unlock()
		if removeErr != nil {
			if !errors.Is(removeErr, os.ErrNotExist) {
				slog.Warn("remove downloaded transcode segment", "component", "playback", "error", removeErr, "segment", segment, "session", opts.SessionID, "playback_session_id", opts.SessionID)
			}
			continue
		}
		removed++
		freedBytes += info.Size()
	}

	s.finishSegmentPrune(generation, floor)
	if removed > 0 {
		slog.Info("pruned downloaded transcode segments",
			"component", "playback",
			"count", removed,
			"freed_bytes", freedBytes,
			"floor_segment", floor,
			"duration_ms", time.Since(started).Milliseconds(),
			"session", opts.SessionID,
			"playback_session_id", opts.SessionID,
		)
	}
}

func (s *TranscodeSession) finishSegmentPrune(generation uint64, floor int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.segmentGeneration {
		return
	}
	if floor > s.lastPruneFloor {
		s.lastPruneFloor = floor
	}
	s.segmentPruneRunning = false
	s.scheduleSegmentPruneLocked()
}
