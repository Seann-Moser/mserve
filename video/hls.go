package video

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"strconv"
	"strings"

	"github.com/grafov/m3u8"
)

// .ts segments, and a preview.jpg into outputDir.
func ConvertToHLS(inputPath, outputDir string, segmentDuration int) error {
	// try GPU first
	if err := ConvertToHLS_GPU(inputPath, outputDir, segmentDuration); err == nil {

		return generatePreview(inputPath, outputDir, time.Duration(rand.Intn(30)+30)*time.Second)
	} else {
		slog.Error("GPU HLS conversion failed, falling back to CPU", "err", err)
	}

	// CPU fallback
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	playlist := filepath.Join(outputDir, "index.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment_%03d.ts")
	args := []string{
		"-i", inputPath,
		"-c:v", "copy", "-c:a", "copy",
		"-hls_time", strconv.Itoa(segmentDuration),
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPattern,
		"-f", "hls", playlist,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("Running FFmpeg (CPU): ffmpeg %v\n", args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	return generatePreview(inputPath, outputDir, time.Duration(rand.Intn(30)+30)*time.Second)
}

// ConvertToHLS_GPU is the same as before…
func ConvertToHLS_GPU(inputPath, outputDir string, segmentDuration int) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}
	playlist := filepath.Join(outputDir, "index.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment_%03d.ts")

	args := []string{
		"-hwaccel", "cuda",
		"-i", inputPath,
		"-c:v", "h264_nvenc",
		"-preset", "p5", "-cq", "19",
		"-c:a", "aac", "-b:a", "128k",
		"-hls_time", strconv.Itoa(segmentDuration),
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPattern,
		"-f", "hls", playlist,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("Running FFmpeg (GPU): ffmpeg %v\n", args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}
	return nil
}
func formatDurationSimpler(d time.Duration) string {
	d = d.Round(time.Second)
	totalSeconds := int(d.Seconds())
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// generatePreview uses FFmpeg to grab a single frame at 1s and save as preview.jpg
func generatePreview(inputPath, outputDir string, duration time.Duration) error {
	previewPath := filepath.Join(outputDir, "preview.jpg")
	if _, err := os.Stat(previewPath); !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(previewPath)
	}
	args := []string{
		"-ss", formatDurationSimpler(duration), // seek to 1 second
		"-i", inputPath, // input file
		"-frames:v", "1", // grab exactly one frame
		"-q:v", "2", // high-quality JPEG (1–31, lower is better)
		previewPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("Generating preview: ffmpeg %v\n", args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("preview generation failed: %w", err)
	}
	return nil
}

func EnsureFFmpegAvailable() error {
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg command not found in PATH: %w. Please ensure ffmpeg is installed and accessible", err)
	}
	return nil
}

// downloadFile downloads to outPath with a progress bar.
// If outPath ends in .mp4 it then runs ffmpeg to compress it in-place.
func downloadFile(ctx context.Context, fileURL, outPath string, v *Video) error {
	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		p := strings.ReplaceAll(outPath, ".mp4", "")
		v.HLS = true
		v.ThumbImage = filepath.Join(p, "preview.jpg")
		v.Src = filepath.Join(p, "index.m3u8")
		d := strings.Split(p, "/")
		v.Dir = d[len(d)-1] + "/" + "index.m3u8"
		v.Preview = filepath.Join(p, "preview.mp4")
		v.Length, _ = GetPlaylistDuration(v.Src)
		return nil

	}
	// 1) Download + progress bar
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	dir := filepath.Dir(outPath)

	// Then, create all necessary directories.
	// MkdirAll does nothing if the directories already exist, which is perfect.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w %s", err, dir)
	}

	// create output file
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create file: %w %s", err, outPath)
	}
	defer func() {
		_ = f.Close()
	}()
	// copy with progress

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	// 2) If it’s an MP4, compress it via ffmpeg
	if strings.EqualFold(filepath.Ext(outPath), ".mp4") {
		p := strings.ReplaceAll(outPath, ".mp4", "")

		err = ConvertToHLS(outPath, p, 6)
		if err != nil {
			return fmt.Errorf("convert to hls: %w", err)
		}
		v.HLS = true
		v.ThumbImage = filepath.Join(p, "preview.jpg")
		v.Src = filepath.Join(p, "index.m3u8")
		d := strings.Split(p, "/")
		v.Dir = d[len(d)-1] + "/" + "index.m3u8"
		v.Preview = filepath.Join(p, "preview.mp4")
		v.Length, _ = GetPlaylistDuration(v.Src)
		_ = CreatePreview(v.Src, v.Preview, 10)
	}

	return nil
}

// GetPlaylistDuration returns the sum of all segment durations in the HLS media playlist.
func GetPlaylistDuration(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
	}()

	p, listType, err := m3u8.DecodeFrom(bufio.NewReader(f), true)
	if err != nil {
		return 0, err
	}
	if listType != m3u8.MEDIA {
		return 0, fmt.Errorf("not a media playlist")
	}

	ml := p.(*m3u8.MediaPlaylist)
	var total float64
	for _, seg := range ml.Segments {
		if seg != nil {
			total += seg.Duration
		}
	}
	return int(total), nil
}

func CreatePreview(playlistPath, outputPath string, clipLength float64) error {
	// seed randomness
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	_, totalDur, err := parsePlaylist(playlistPath)
	if err != nil {
		return fmt.Errorf("parse playlist: %w", err)
	}

	// split into three equal sections
	sections := [][2]float64{
		{0, totalDur / 3},
		{totalDur / 3, 2 * totalDur / 3},
		{2 * totalDur / 3, totalDur},
	}

	// use an ephemeral temp dir
	tempDir, err := os.MkdirTemp("", "hls-preview-*")
	if err != nil {
		return fmt.Errorf("make temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()
	var clipFiles []string
	for i, sec := range sections {
		secStart, secEnd := sec[0], sec[1]
		if secEnd-secStart < clipLength {
			return fmt.Errorf("section %d too short for clip length %.1f", i, clipLength)
		}
		// choose random offset
		start := secStart + r.Float64()*(secEnd-secStart-clipLength)
		outFile := filepath.Join(tempDir, fmt.Sprintf("clip_%d.ts", i))
		if err := extractClip(context.Background(), playlistPath, start, clipLength, outFile); err != nil {
			return fmt.Errorf("extract clip %d: %w", i, err)
		}
		clipFiles = append(clipFiles, outFile)
	}

	// write ffmpeg concat list
	listFile := filepath.Join(tempDir, "list.txt")
	if err := writeConcatList(listFile, clipFiles); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}

	// stitch them together
	if err := concatClips(context.Background(), listFile, outputPath); err != nil {
		return fmt.Errorf("concat clips: %w", err)
	}

	return nil
}

func parsePlaylist(path string) ([]*m3u8.MediaSegment, float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = f.Close()
	}()

	p, listType, err := m3u8.DecodeFrom(bufio.NewReader(f), true)
	if err != nil {
		return nil, 0, err
	}
	if listType != m3u8.MEDIA {
		return nil, 0, fmt.Errorf("not a media playlist")
	}
	ml := p.(*m3u8.MediaPlaylist)
	var total float64
	for _, seg := range ml.Segments {
		if seg != nil {
			total += seg.Duration
		}
	}
	return ml.Segments, total, nil
}

func extractClip(ctx context.Context, playlist string, offset, length float64, out string) error {
	// rely on ffmpeg being in your PATH
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", fmt.Sprintf("%.3f", offset),
		"-i", playlist,
		"-t", fmt.Sprintf("%.3f", length),
		"-c", "copy",
		"-y",
		out,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeConcatList(path string, clips []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	for _, c := range clips {
		_, _ = fmt.Fprintf(f, "file '%s'\n", c)
	}
	return nil
}

func concatClips(ctx context.Context, listFile, out string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		"-y",
		out,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// downloadHLS downloads an HLS (.m3u8) playlist into a single file by
// concatenating all the .ts segments.
func downloadHLS(ctx context.Context, playlistURL, outPath string) error {
	// fetch playlist
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, playlistURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch playlist: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	pl, listType, err := m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		return fmt.Errorf("parse playlist: %w", err)
	}
	if listType != m3u8.MEDIA {
		return errors.New("not a media playlist")
	}
	media := pl.(*m3u8.MediaPlaylist)

	// prepare output
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer func() {
		_ = outFile.Close()
	}()

	// download each segment
	for _, seg := range media.Segments {
		if seg == nil {
			continue
		}
		segURL := resolveURL(playlistURL, seg.URI)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, segURL, nil)
		r2, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch segment %s: %w", segURL, err)
		}
		if r2.StatusCode != http.StatusOK {
			_ = r2.Body.Close()
			return fmt.Errorf("bad status for segment: %d", r2.StatusCode)
		}
		// append segment data
		if _, err := io.Copy(outFile, r2.Body); err != nil {
			_ = r2.Body.Close()
			return fmt.Errorf("write segment: %w", err)
		}
		_ = r2.Body.Close()
	}

	return nil
}

func resolveURL(basePage, href string) string {
	base, err := url.Parse(basePage)
	if err != nil {
		return href // fallback
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}
