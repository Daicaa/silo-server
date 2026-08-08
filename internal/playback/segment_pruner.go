package playback

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	segmentPruneHysteresis = 5
	segmentPruneBatchSize  = 512
	segmentPruneRetryDelay = 5 * time.Second
)

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
// It visits each newly expired segment number at most once in bounded batches,
// avoiding repeated full-directory scans when FFmpeg has generated far ahead
// of the client. The current process's startup window remains present so real
// manifest reloads cannot get stuck behind startupFilesReady after cleanup.
func (s *TranscodeSession) pruneDownloadedSegments(generation uint64, floor int) {
	started := time.Now()
	s.mu.Lock()
	if generation != s.segmentGeneration || s.restarting {
		s.mu.Unlock()
		return
	}
	opts := s.opts
	outputDir := s.outputDir
	fromFloor := s.lastPruneFloor
	s.mu.Unlock()

	segmentDuration := opts.SegmentDuration
	if segmentDuration <= 0 {
		segmentDuration = defaultSegmentDuration
	}
	freshGuard := 2*time.Duration(segmentDuration)*time.Second + 30*time.Second
	startupEnd := opts.StartSegmentNumber + startupSegmentRequirement(opts)
	fromSegment := max(fromFloor, startupEnd)
	toSegment := min(floor, fromSegment+segmentPruneBatchSize)
	if fromSegment >= toSegment {
		s.finishSegmentPrune(generation, floor, floor, 0)
		return
	}

	if _, err := os.Stat(outputDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.finishSegmentPrune(generation, toSegment, floor, 0)
			return
		}
		slog.Warn("stat transcode directory for pruning", "component", "playback", "error", err, "session", opts.SessionID, "playback_session_id", opts.SessionID)
		s.finishSegmentPrune(generation, fromSegment, floor, segmentPruneRetryDelay)
		return
	}

	processedFloor := toSegment
	var retryAfter time.Duration
	removed := 0
	var freedBytes int64
	for segment := fromSegment; segment < toSegment; segment++ {
		path := filepath.Join(outputDir, segmentFilename(segment, opts))
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			processedFloor, retryAfter = earlierPruneRetry(processedFloor, retryAfter, segment, segmentPruneRetryDelay)
			slog.Warn("stat downloaded transcode segment", "component", "playback", "error", err, "segment", segment, "session", opts.SessionID, "playback_session_id", opts.SessionID)
			continue
		}
		freshUntil := info.ModTime().Add(freshGuard)
		if delay := time.Until(freshUntil); delay > 0 {
			processedFloor, retryAfter = earlierPruneRetry(processedFloor, retryAfter, segment, delay)
			continue
		}

		// Serialize the unlink with restart and shutdown generation changes.
		// Once either advances the generation, this pass cannot delete files
		// generated for a replacement timeline or session object.
		s.mu.Lock()
		if generation != s.segmentGeneration || s.restarting {
			s.mu.Unlock()
			return
		}
		removeErr := os.Remove(path)
		s.mu.Unlock()
		if removeErr != nil {
			if !errors.Is(removeErr, os.ErrNotExist) {
				processedFloor, retryAfter = earlierPruneRetry(processedFloor, retryAfter, segment, segmentPruneRetryDelay)
				slog.Warn("remove downloaded transcode segment", "component", "playback", "error", removeErr, "segment", segment, "session", opts.SessionID, "playback_session_id", opts.SessionID)
			}
			continue
		}
		removed++
		freedBytes += info.Size()
	}

	s.finishSegmentPrune(generation, processedFloor, floor, retryAfter)
	if removed > 0 {
		slog.Info("pruned downloaded transcode segments",
			"component", "playback",
			"count", removed,
			"freed_bytes", freedBytes,
			"floor_segment", processedFloor,
			"duration_ms", time.Since(started).Milliseconds(),
			"session", opts.SessionID,
			"playback_session_id", opts.SessionID,
		)
	}
}

func earlierPruneRetry(currentFloor int, currentDelay time.Duration, segment int, delay time.Duration) (int, time.Duration) {
	if segment < currentFloor {
		return segment, max(delay, time.Millisecond)
	}
	return currentFloor, currentDelay
}

func (s *TranscodeSession) finishSegmentPrune(generation uint64, processedFloor, targetFloor int, retryAfter time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.segmentGeneration {
		return
	}
	if processedFloor > s.lastPruneFloor {
		s.lastPruneFloor = processedFloor
	}
	if retryAfter > 0 {
		time.AfterFunc(retryAfter, func() {
			s.mu.Lock()
			if generation != s.segmentGeneration || s.restarting {
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
			go s.pruneDownloadedSegments(generation, targetFloor)
		})
		return
	}
	s.segmentPruneRunning = false
	s.scheduleSegmentPruneLocked()
}
