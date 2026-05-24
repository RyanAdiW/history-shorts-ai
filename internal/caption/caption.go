package caption

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	minWordsPerCaption = 3
	maxWordsPerCaption = 8
)

type Status string

const (
	StatusGenerated Status = "generated"
	StatusReused    Status = "reused"
)

type Config struct {
	ScriptPath string
	AudioPath  string
	OutputPath string
	Force      bool
	Logger     *slog.Logger
}

type Result struct {
	Path     string
	Status   Status
	Chunks   int
	Duration time.Duration
}

func GenerateFromFiles(config Config) (Result, error) {
	logger := loggerOrDefault(config.Logger)
	outputPath := strings.TrimSpace(config.OutputPath)
	if outputPath == "" {
		return Result{}, errors.New("captions output path is empty")
	}

	if !config.Force && fileExists(outputPath) {
		logger.Info("reused captions", "output_file", outputPath)
		return Result{Path: outputPath, Status: StatusReused}, nil
	}

	scriptPath := strings.TrimSpace(config.ScriptPath)
	if scriptPath == "" {
		return Result{}, errors.New("script.txt path is empty")
	}
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("script.txt is missing at %s", scriptPath)
		}
		return Result{}, fmt.Errorf("read script.txt: %w", err)
	}
	script := strings.TrimSpace(string(scriptBytes))
	if script == "" {
		return Result{}, errors.New("script.txt is empty")
	}

	audioPath := strings.TrimSpace(config.AudioPath)
	if audioPath == "" {
		return Result{}, errors.New("voice.mp3 path is empty")
	}
	if _, err := os.Stat(audioPath); err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("voice.mp3 is missing at %s", audioPath)
		}
		return Result{}, fmt.Errorf("inspect voice.mp3: %w", err)
	}

	duration, err := MP3Duration(audioPath)
	if err != nil {
		return Result{}, fmt.Errorf("estimate voice.mp3 duration: %w", err)
	}

	srt, chunks, err := GenerateSRT(script, duration)
	if err != nil {
		return Result{}, err
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create captions output directory %s: %w", dir, err)
	}
	if err := os.WriteFile(outputPath, []byte(srt), 0o644); err != nil {
		return Result{}, fmt.Errorf("write captions.srt: %w", err)
	}

	logger.Info("generated captions", "output_file", outputPath, "chunks", chunks, "duration", duration.String())
	return Result{Path: outputPath, Status: StatusGenerated, Chunks: chunks, Duration: duration}, nil
}

func GenerateSRT(script string, audioDuration time.Duration) (string, int, error) {
	script = normalizeWhitespace(script)
	if script == "" {
		return "", 0, errors.New("script.txt is empty")
	}
	if audioDuration <= 0 {
		return "", 0, fmt.Errorf("voice.mp3 duration must be greater than zero, got %s", audioDuration)
	}

	chunks := SplitScript(script)
	if len(chunks) == 0 {
		return "", 0, errors.New("script.txt did not produce any caption chunks")
	}

	weights := make([]int, len(chunks))
	totalWeight := 0
	for i, chunk := range chunks {
		weight := len([]rune(chunk))
		if weight < 1 {
			weight = 1
		}
		weights[i] = weight
		totalWeight += weight
	}

	var b strings.Builder
	var start time.Duration
	var targetEnd float64
	durationNS := float64(audioDuration)
	for i, chunk := range chunks {
		if i == len(chunks)-1 {
			targetEnd = durationNS
		} else {
			targetEnd += durationNS * float64(weights[i]) / float64(totalWeight)
		}
		end := time.Duration(math.Round(targetEnd))
		if end <= start {
			end = start + time.Millisecond
		}

		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, formatTimestamp(start), formatTimestamp(end), chunk)
		start = end
	}

	return b.String(), len(chunks), nil
}

func SplitScript(script string) []string {
	words := strings.Fields(normalizeWhitespace(script))
	if len(words) == 0 {
		return nil
	}

	var chunks []string
	current := make([]string, 0, maxWordsPerCaption)
	for _, word := range words {
		current = append(current, word)
		if shouldCloseChunk(word, len(current)) {
			chunks = append(chunks, strings.Join(current, " "))
			current = make([]string, 0, maxWordsPerCaption)
		}
	}
	if len(current) > 0 {
		if len(current) < minWordsPerCaption && len(chunks) > 0 {
			lastWords := strings.Fields(chunks[len(chunks)-1])
			if len(lastWords)+len(current) <= maxWordsPerCaption {
				chunks[len(chunks)-1] = chunks[len(chunks)-1] + " " + strings.Join(current, " ")
				return chunks
			}
		}
		chunks = append(chunks, strings.Join(current, " "))
	}
	return chunks
}

func MP3Duration(path string) (time.Duration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return 0, errors.New("voice.mp3 is empty")
	}

	offset := id3v2Offset(data)
	var duration time.Duration
	frames := 0
	for i := offset; i+4 <= len(data); {
		frame, ok := parseMP3Frame(data[i:])
		if !ok {
			i++
			continue
		}
		duration += time.Duration(float64(time.Second) * float64(frame.samples) / float64(frame.sampleRate))
		frames++
		i += frame.length
	}
	if frames == 0 {
		return 0, errors.New("no MP3 audio frames found")
	}
	return duration, nil
}

type mp3Frame struct {
	length     int
	samples    int
	sampleRate int
}

func parseMP3Frame(data []byte) (mp3Frame, bool) {
	if len(data) < 4 {
		return mp3Frame{}, false
	}
	header := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	if header&0xffe00000 != 0xffe00000 {
		return mp3Frame{}, false
	}

	versionBits := int((header >> 19) & 0x3)
	layerBits := int((header >> 17) & 0x3)
	bitrateIndex := int((header >> 12) & 0xf)
	sampleRateIndex := int((header >> 10) & 0x3)
	padding := int((header >> 9) & 0x1)

	if versionBits == 1 || layerBits == 0 || bitrateIndex == 0 || bitrateIndex == 15 || sampleRateIndex == 3 {
		return mp3Frame{}, false
	}

	bitrate := bitrateKbps(versionBits, layerBits, bitrateIndex) * 1000
	sampleRate := sampleRate(versionBits, sampleRateIndex)
	if bitrate == 0 || sampleRate == 0 {
		return mp3Frame{}, false
	}

	samples := samplesPerFrame(versionBits, layerBits)
	length := frameLength(versionBits, layerBits, bitrate, sampleRate, padding)
	if samples == 0 || length < 4 || length > len(data) {
		return mp3Frame{}, false
	}

	return mp3Frame{length: length, samples: samples, sampleRate: sampleRate}, true
}

func bitrateKbps(versionBits int, layerBits int, index int) int {
	mpeg1 := versionBits == 3
	if mpeg1 {
		switch layerBits {
		case 3:
			return []int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}[index]
		case 2:
			return []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}[index]
		case 1:
			return []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}[index]
		}
	}

	switch layerBits {
	case 3:
		return []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}[index]
	case 2, 1:
		return []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}[index]
	default:
		return 0
	}
}

func sampleRate(versionBits int, index int) int {
	switch versionBits {
	case 3:
		return []int{44100, 48000, 32000}[index]
	case 2:
		return []int{22050, 24000, 16000}[index]
	case 0:
		return []int{11025, 12000, 8000}[index]
	default:
		return 0
	}
}

func samplesPerFrame(versionBits int, layerBits int) int {
	switch layerBits {
	case 3:
		return 384
	case 2:
		return 1152
	case 1:
		if versionBits == 3 {
			return 1152
		}
		return 576
	default:
		return 0
	}
}

func frameLength(versionBits int, layerBits int, bitrate int, sampleRate int, padding int) int {
	if layerBits == 3 {
		return ((12 * bitrate / sampleRate) + padding) * 4
	}
	coefficient := 144
	if layerBits == 1 && versionBits != 3 {
		coefficient = 72
	}
	return coefficient*bitrate/sampleRate + padding
}

func id3v2Offset(data []byte) int {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return 0
	}
	size := syncSafeInt(data[6:10])
	offset := 10 + size
	if data[5]&0x10 != 0 {
		offset += 10
	}
	if offset >= len(data) {
		return 0
	}
	return offset
}

func syncSafeInt(data []byte) int {
	if len(data) != 4 {
		return 0
	}
	return int(data[0]&0x7f)<<21 | int(data[1]&0x7f)<<14 | int(data[2]&0x7f)<<7 | int(data[3]&0x7f)
}

func shouldCloseChunk(word string, wordCount int) bool {
	if wordCount < minWordsPerCaption {
		return false
	}
	if wordCount >= maxWordsPerCaption {
		return true
	}
	if endsWithAny(word, ".!?") {
		return true
	}
	return wordCount >= 5 && endsWithAny(word, ",;:")
}

func endsWithAny(word string, suffixes string) bool {
	word = strings.TrimRight(word, "\"')]} ")
	if word == "" {
		return false
	}
	return strings.ContainsRune(suffixes, rune(word[len(word)-1]))
}

func formatTimestamp(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	totalMillis := duration.Round(time.Millisecond).Milliseconds()
	hours := totalMillis / 3_600_000
	totalMillis %= 3_600_000
	minutes := totalMillis / 60_000
	totalMillis %= 60_000
	seconds := totalMillis / 1_000
	millis := totalMillis % 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
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
