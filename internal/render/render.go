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
	EnvVoiceVolume     = "VIDEO_VOICE_VOLUME"
	DefaultVoiceVolume = 2.0
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
	ImagesDir     string
	AudioPath     string
	CaptionsPath  string
	RawOutputPath string
	OutputPath    string
	Force         bool
	FFmpegPath    string
	FFprobePath   string
	VoiceVolume   float64
	Logger        *slog.Logger
}

type Result struct {
	Path       string
	Status     Status
	Raw        StepResult
	Final      StepResult
	Images     int
	Duration   time.Duration
	FinalReady bool
}

type StepResult struct {
	Path   string
	Status Status
}

type rawInputFiles struct {
	images     []string
	audioPath  string
	outputPath string
	outputDir  string
}

type finalInputFiles struct {
	rawPath      string
	captionsPath string
	outputPath   string
	outputDir    string
}

func RenderFromFiles(ctx context.Context, config Config) (Result, error) {
	logger := loggerOrDefault(config.Logger)

	rawResult, rawFiles, duration, err := RenderRawFromFiles(ctx, config)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Path:     rawResult.Path,
		Status:   rawResult.Status,
		Raw:      rawResult,
		Images:   len(rawFiles.images),
		Duration: duration,
	}

	captionsPath := strings.TrimSpace(config.CaptionsPath)
	if captionsPath == "" || !fileExists(captionsPath) {
		logger.Info("skipped final video render", "reason", "captions.srt is missing", "captions_file", captionsPath)
		return result, nil
	}

	finalResult, err := RenderFinalFromFiles(ctx, config)
	if err != nil {
		return Result{}, err
	}
	result.Path = finalResult.Path
	result.Status = finalResult.Status
	result.Final = finalResult
	result.FinalReady = true
	return result, nil
}

func RenderRawFromFiles(ctx context.Context, config Config) (StepResult, rawInputFiles, time.Duration, error) {
	logger := loggerOrDefault(config.Logger)

	files, err := validateRawInputs(config)
	if err != nil {
		return StepResult{}, rawInputFiles{}, 0, err
	}

	if !config.Force && fileExists(files.outputPath) {
		logger.Info("reused raw video", "output_file", files.outputPath)
		duration, err := ProbeDuration(ctx, commandOrDefault(config.FFprobePath, defaultFFprobePath), files.audioPath)
		if err != nil {
			return StepResult{}, rawInputFiles{}, 0, fmt.Errorf("probe voice.mp3 duration: %w", err)
		}
		return StepResult{Path: files.outputPath, Status: StatusReused}, files, duration, nil
	}

	voiceVolume, err := voiceVolumeFromConfig(config.VoiceVolume)
	if err != nil {
		return StepResult{}, rawInputFiles{}, 0, err
	}
	logger.Info("using render voice volume", "voice_volume", voiceVolume)

	duration, err := ProbeDuration(ctx, commandOrDefault(config.FFprobePath, defaultFFprobePath), files.audioPath)
	if err != nil {
		return StepResult{}, rawInputFiles{}, 0, fmt.Errorf("probe voice.mp3 duration: %w", err)
	}
	durationPerImage := duration / time.Duration(len(files.images))
	if durationPerImage <= 0 {
		return StepResult{}, rawInputFiles{}, 0, fmt.Errorf("duration per image must be greater than zero, got %s", durationPerImage)
	}
	framesPerImage := max(1, int(math.Ceil(durationPerImage.Seconds()*outputFPS)))

	concatPath, err := writeConcatFile(files.outputDir, files.images)
	if err != nil {
		return StepResult{}, rawInputFiles{}, 0, err
	}

	args := buildRawFFmpegArgs(
		filepath.Base(concatPath),
		filepath.Base(files.audioPath),
		filepath.Base(files.outputPath),
		framesPerImage,
		duration,
		voiceVolume,
	)
	if err := runCommand(ctx, files.outputDir, commandOrDefault(config.FFmpegPath, defaultFFmpegPath), args); err != nil {
		return StepResult{}, rawInputFiles{}, 0, fmt.Errorf("run ffmpeg raw render: %w", err)
	}
	if err := os.Remove(concatPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove temporary render concat file", "path", concatPath, "error", err)
	}

	logger.Info(
		"generated raw video",
		"output_file", files.outputPath,
		"images", len(files.images),
		"duration", duration.String(),
		"duration_per_image", durationPerImage.String(),
		"voice_volume", voiceVolume,
	)
	return StepResult{Path: files.outputPath, Status: StatusGenerated}, files, duration, nil
}

func RenderFinalFromFiles(ctx context.Context, config Config) (StepResult, error) {
	logger := loggerOrDefault(config.Logger)

	files, err := validateFinalInputs(config)
	if err != nil {
		return StepResult{}, err
	}

	if !config.Force && fileExists(files.outputPath) {
		logger.Info("reused final video", "output_file", files.outputPath)
		return StepResult{Path: files.outputPath, Status: StatusReused}, nil
	}

	args := buildFinalFFmpegArgs(
		filepath.Base(files.rawPath),
		filepath.Base(files.captionsPath),
		filepath.Base(files.outputPath),
	)
	if err := runCommand(ctx, files.outputDir, commandOrDefault(config.FFmpegPath, defaultFFmpegPath), args); err != nil {
		return StepResult{}, fmt.Errorf("run ffmpeg final render: %w", err)
	}

	logger.Info("generated final video", "input_file", files.rawPath, "captions_file", files.captionsPath, "output_file", files.outputPath)
	return StepResult{Path: files.outputPath, Status: StatusGenerated}, nil
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

func validateRawInputs(config Config) (rawInputFiles, error) {
	outputPath := strings.TrimSpace(config.RawOutputPath)
	if outputPath == "" {
		return rawInputFiles{}, errors.New("raw.mp4 output path is empty")
	}
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		return rawInputFiles{}, fmt.Errorf("raw.mp4 output path %s is a directory", outputPath)
	} else if err != nil && !os.IsNotExist(err) {
		return rawInputFiles{}, fmt.Errorf("inspect raw.mp4 output path %s: %w", outputPath, err)
	}

	images, err := findImages(config.ImagesDir)
	if err != nil {
		return rawInputFiles{}, err
	}

	audioPath := strings.TrimSpace(config.AudioPath)
	if audioPath == "" {
		return rawInputFiles{}, errors.New("voice.mp3 path is empty")
	}
	if err := requireFile(audioPath, "voice.mp3"); err != nil {
		return rawInputFiles{}, err
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return rawInputFiles{}, fmt.Errorf("create raw render output directory %s: %w", outputDir, err)
	}

	return rawInputFiles{
		images:     images,
		audioPath:  filepath.Clean(audioPath),
		outputPath: filepath.Clean(outputPath),
		outputDir:  outputDir,
	}, nil
}

func validateFinalInputs(config Config) (finalInputFiles, error) {
	rawPath := strings.TrimSpace(config.RawOutputPath)
	if rawPath == "" {
		return finalInputFiles{}, errors.New("raw.mp4 path is empty")
	}
	if err := requireFile(rawPath, "raw.mp4"); err != nil {
		return finalInputFiles{}, err
	}

	captionsPath := strings.TrimSpace(config.CaptionsPath)
	if captionsPath == "" {
		return finalInputFiles{}, errors.New("captions.srt path is empty")
	}
	if err := requireFile(captionsPath, "captions.srt"); err != nil {
		return finalInputFiles{}, err
	}

	outputPath := strings.TrimSpace(config.OutputPath)
	if outputPath == "" {
		return finalInputFiles{}, errors.New("final.mp4 output path is empty")
	}
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		return finalInputFiles{}, fmt.Errorf("final.mp4 output path %s is a directory", outputPath)
	} else if err != nil && !os.IsNotExist(err) {
		return finalInputFiles{}, fmt.Errorf("inspect final.mp4 output path %s: %w", outputPath, err)
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return finalInputFiles{}, fmt.Errorf("create final render output directory %s: %w", outputDir, err)
	}

	return finalInputFiles{
		rawPath:      filepath.Clean(rawPath),
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

func buildRawFFmpegArgs(concatFile string, audioFile string, outputFile string, framesPerImage int, duration time.Duration, voiceVolume float64) []string {
	videoFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,zoompan=z='min(zoom+0.0004,1.08)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=%d,format=yuv420p",
		outputWidth,
		outputHeight,
		outputWidth,
		outputHeight,
		framesPerImage,
		outputWidth,
		outputHeight,
		outputFPS,
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
		"-af", fmt.Sprintf("volume=%s", formatVolume(voiceVolume)),
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

func buildFinalFFmpegArgs(rawFile string, captionsFile string, outputFile string) []string {
	videoFilter := fmt.Sprintf("subtitles=%s,format=yuv420p", escapeFilterPath(captionsFile))
	return []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", rawFile,
		"-vf", videoFilter,
		"-map", "0:v:0",
		"-map", "0:a:0",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-movflags", "+faststart",
		outputFile,
	}
}

func formatSeconds(duration time.Duration) string {
	return fmt.Sprintf("%.3f", duration.Seconds())
}

func VoiceVolumeFromEnv() (float64, error) {
	return parseVoiceVolume(os.Getenv(EnvVoiceVolume))
}

func parseVoiceVolume(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultVoiceVolume, nil
	}

	volume, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive number; got %q", EnvVoiceVolume, value)
	}
	if volume <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0; got %s", EnvVoiceVolume, value)
	}
	return volume, nil
}

func voiceVolumeFromConfig(value float64) (float64, error) {
	if value != 0 {
		if value <= 0 {
			return 0, fmt.Errorf("render voice volume must be greater than 0; got %s", formatVolume(value))
		}
		return value, nil
	}
	return VoiceVolumeFromEnv()
}

func formatVolume(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
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
