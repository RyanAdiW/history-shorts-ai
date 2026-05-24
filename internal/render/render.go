package render

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultFFmpegPath  = "ffmpeg"
	defaultFFprobePath = "ffprobe"
	outputWidth        = 1080
	outputHeight       = 1920
	outputFPS          = 30
)

type Status string

const (
	StatusGenerated Status = "generated"
	StatusReused    Status = "reused"
)

type Config struct {
	ImagesDir    string
	AudioPath    string
	CaptionsPath string
	OutputPath   string
	Force        bool
	FFmpegPath   string
	FFprobePath  string
	Logger       *slog.Logger
}

type Result struct {
	Path             string
	Status           Status
	Images           int
	Duration         time.Duration
	DurationPerImage time.Duration
}

type inputFiles struct {
	images       []string
	audioPath    string
	captionsPath string
	outputPath   string
	outputDir    string
}

func RenderFromFiles(ctx context.Context, config Config) (Result, error) {
	logger := loggerOrDefault(config.Logger)

	files, err := validateInputs(config)
	if err != nil {
		return Result{}, err
	}

	if !config.Force && fileExists(files.outputPath) {
		logger.Info("reused rendered video", "output_file", files.outputPath)
		return Result{Path: files.outputPath, Status: StatusReused, Images: len(files.images)}, nil
	}

	duration, err := ProbeDuration(ctx, commandOrDefault(config.FFprobePath, defaultFFprobePath), files.audioPath)
	if err != nil {
		return Result{}, fmt.Errorf("probe voice.mp3 duration: %w", err)
	}
	durationPerImage := duration / time.Duration(len(files.images))
	if durationPerImage <= 0 {
		return Result{}, fmt.Errorf("duration per image must be greater than zero, got %s", durationPerImage)
	}
	framesPerImage := max(1, int(math.Ceil(durationPerImage.Seconds()*outputFPS)))

	concatPath, err := writeConcatFile(files.outputDir, files.images)
	if err != nil {
		return Result{}, err
	}

	args := buildFFmpegArgs(
		filepath.Base(concatPath),
		filepath.Base(files.audioPath),
		filepath.Base(files.captionsPath),
		filepath.Base(files.outputPath),
		framesPerImage,
		duration,
	)
	if err := runCommand(ctx, files.outputDir, commandOrDefault(config.FFmpegPath, defaultFFmpegPath), args); err != nil {
		return Result{}, fmt.Errorf("run ffmpeg: %w", err)
	}
	if err := os.Remove(concatPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove temporary render concat file", "path", concatPath, "error", err)
	}

	logger.Info(
		"generated rendered video",
		"output_file", files.outputPath,
		"images", len(files.images),
		"duration", duration.String(),
		"duration_per_image", durationPerImage.String(),
	)
	return Result{
		Path:             files.outputPath,
		Status:           StatusGenerated,
		Images:           len(files.images),
		Duration:         duration,
		DurationPerImage: durationPerImage,
	}, nil
}

func ProbeDuration(ctx context.Context, ffprobePath string, audioPath string) (time.Duration, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath,
	}
	output, err := exec.CommandContext(ctx, ffprobePath, args...).CombinedOutput()
	if err != nil {
		return 0, commandError(ffprobePath, err, output)
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", strings.TrimSpace(string(output)), err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("ffprobe duration must be greater than zero, got %.3f", seconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func validateInputs(config Config) (inputFiles, error) {
	outputPath := strings.TrimSpace(config.OutputPath)
	if outputPath == "" {
		return inputFiles{}, errors.New("final.mp4 output path is empty")
	}
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		return inputFiles{}, fmt.Errorf("final.mp4 output path %s is a directory", outputPath)
	} else if err != nil && !os.IsNotExist(err) {
		return inputFiles{}, fmt.Errorf("inspect final.mp4 output path %s: %w", outputPath, err)
	}

	images, err := findImages(config.ImagesDir)
	if err != nil {
		return inputFiles{}, err
	}

	audioPath := strings.TrimSpace(config.AudioPath)
	if audioPath == "" {
		return inputFiles{}, errors.New("voice.mp3 path is empty")
	}
	if err := requireFile(audioPath, "voice.mp3"); err != nil {
		return inputFiles{}, err
	}

	captionsPath := strings.TrimSpace(config.CaptionsPath)
	if captionsPath == "" {
		return inputFiles{}, errors.New("captions.srt path is empty")
	}
	if err := requireFile(captionsPath, "captions.srt"); err != nil {
		return inputFiles{}, err
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return inputFiles{}, fmt.Errorf("create render output directory %s: %w", outputDir, err)
	}

	return inputFiles{
		images:       images,
		audioPath:    filepath.Clean(audioPath),
		captionsPath: filepath.Clean(captionsPath),
		outputPath:   filepath.Clean(outputPath),
		outputDir:    outputDir,
	}, nil
}

func findImages(imagesDir string) ([]string, error) {
	imagesDir = strings.TrimSpace(imagesDir)
	if imagesDir == "" {
		return nil, errors.New("images directory path is empty")
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("images directory is missing at %s", imagesDir)
		}
		return nil, fmt.Errorf("read images directory %s: %w", imagesDir, err)
	}

	var images []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			images = append(images, filepath.Join(imagesDir, entry.Name()))
		}
	}
	sort.Strings(images)
	if len(images) == 0 {
		return nil, fmt.Errorf("no PNG images found in %s", imagesDir)
	}
	return images, nil
}

func requireFile(path string, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s is missing at %s", name, path)
		}
		return fmt.Errorf("inspect %s at %s: %w", name, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s path %s is a directory", name, path)
	}
	return nil
}

func writeConcatFile(outputDir string, images []string) (string, error) {
	tempFile, err := os.CreateTemp(outputDir, ".render-*.concat")
	if err != nil {
		return "", fmt.Errorf("create temporary render concat file: %w", err)
	}
	tempPath := tempFile.Name()

	for _, image := range images {
		relative, err := filepath.Rel(outputDir, image)
		if err != nil {
			tempFile.Close()
			os.Remove(tempPath)
			return "", fmt.Errorf("calculate relative image path for %s: %w", image, err)
		}
		if _, err := fmt.Fprintf(tempFile, "file '%s'\n", escapeConcatPath(relative)); err != nil {
			tempFile.Close()
			os.Remove(tempPath)
			return "", fmt.Errorf("write temporary render concat file: %w", err)
		}
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("close temporary render concat file: %w", err)
	}
	return tempPath, nil
}

func buildFFmpegArgs(concatFile string, audioFile string, captionsFile string, outputFile string, framesPerImage int, duration time.Duration) []string {
	videoFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,zoompan=z='min(zoom+0.0004,1.08)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=%d,subtitles=%s,format=yuv420p",
		outputWidth,
		outputHeight,
		outputWidth,
		outputHeight,
		framesPerImage,
		outputWidth,
		outputHeight,
		outputFPS,
		escapeFilterPath(captionsFile),
	)

	return []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile,
		"-i", audioFile,
		"-vf", videoFilter,
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-r", strconv.Itoa(outputFPS),
		"-t", formatSeconds(duration),
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		"-movflags", "+faststart",
		outputFile,
	}
}

func formatSeconds(duration time.Duration) string {
	return fmt.Sprintf("%.3f", duration.Seconds())
}

func runCommand(ctx context.Context, dir string, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return commandError(name, err, output)
	}
	return nil
}

func commandError(name string, err error, output []byte) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return fmt.Errorf("%s is required but was not found in PATH", name)
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func escapeConcatPath(path string) string {
	path = filepath.ToSlash(path)
	return strings.ReplaceAll(path, "'", "'\\''")
}

func escapeFilterPath(path string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		":", "\\:",
		"'", "\\'",
		",", "\\,",
		"[", "\\[",
		"]", "\\]",
	)
	return replacer.Replace(filepath.ToSlash(path))
}

func commandOrDefault(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
