package caption

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const (
	DefaultTranscriptionModel = "gpt-4o-mini-transcribe"

	requestTimeout     = 10 * time.Minute
	minWordsPerCaption = 3
	maxWordsPerCaption = 8
)

type Status string

const (
	StatusGenerated Status = "generated"
	StatusReused    Status = "reused"
)

type Config struct {
	ScriptPath               string
	AudioPath                string
	OutputPath               string
	OpenAIAPIKey             string
	OpenAITranscriptionModel string
	Transcriber              Transcriber
	Force                    bool
	Logger                   *slog.Logger
}

type Result struct {
	Path     string
	Status   Status
	Chunks   int
	Duration time.Duration
	Source   string
}

type Transcript struct {
	Text     string
	Segments []Segment
}

type Segment struct {
	Text  string
	Start time.Duration
	End   time.Duration
}

type Caption struct {
	Text  string
	Start time.Duration
	End   time.Duration
}

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string, prompt string) (Transcript, error)
}

type OpenAITranscriber struct {
	model  string
	api    openai.Client
	logger *slog.Logger
}

func NewOpenAITranscriber(apiKey string, model string, logger *slog.Logger) OpenAITranscriber {
	return OpenAITranscriber{
		model:  valueOrDefault(model, DefaultTranscriptionModel),
		api:    openai.NewClient(option.WithAPIKey(apiKey)),
		logger: loggerOrDefault(logger),
	}
}

func (t OpenAITranscriber) Transcribe(ctx context.Context, audioPath string, prompt string) (Transcript, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return Transcript{}, fmt.Errorf("open media for transcription: %w", err)
	}
	defer file.Close()

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	params := openai.AudioTranscriptionNewParams{
		File:                   openai.File(file, filepath.Base(audioPath), transcriptionContentType(audioPath)),
		Model:                  openai.AudioModel(t.model),
		ResponseFormat:         openai.AudioResponseFormatJSON,
		TimestampGranularities: []string{"segment"},
	}
	if prompt = strings.TrimSpace(prompt); prompt != "" {
		params.Prompt = openai.String(prompt)
	}

	resp, err := t.api.Audio.Transcriptions.New(requestCtx, params)
	if err != nil {
		t.logger.Error("OpenAI transcription request failed", "model", t.model, "audio_path", audioPath, "error", err)
		return Transcript{}, err
	}

	transcript := Transcript{Text: strings.TrimSpace(resp.Text)}
	for _, segment := range resp.Segments {
		transcript.Segments = append(transcript.Segments, Segment{
			Text:  strings.TrimSpace(segment.Text),
			Start: secondsToDuration(segment.Start),
			End:   secondsToDuration(segment.End),
		})
	}
	return transcript, nil
}

func transcriptionContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4":
		return "video/mp4"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".webm":
		return "audio/webm"
	case ".mp3", ".mpeg", ".mpga":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func GenerateFromFiles(ctx context.Context, config Config) (Result, error) {
	logger := loggerOrDefault(config.Logger)
	outputPath := strings.TrimSpace(config.OutputPath)
	if outputPath == "" {
		return Result{}, errors.New("captions output path is empty")
	}

	if !config.Force && fileExists(outputPath) {
		logger.Info("reused captions", "output_file", outputPath)
		return Result{Path: outputPath, Status: StatusReused}, nil
	}

	audioPath := strings.TrimSpace(config.AudioPath)
	if audioPath == "" {
		return Result{}, errors.New("caption transcription source path is empty")
	}
	if _, err := os.Stat(audioPath); err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("caption transcription source is missing at %s", audioPath)
		}
		return Result{}, fmt.Errorf("inspect caption transcription source: %w", err)
	}

	script, err := readOptionalScript(config.ScriptPath)
	if err != nil {
		logger.Warn("script fallback is unavailable", "script_path", config.ScriptPath, "error", err)
	}

	if transcriber := transcriberFromConfig(config, logger); transcriber != nil {
		transcript, err := transcriber.Transcribe(ctx, audioPath, script)
		if err != nil {
			return Result{}, fmt.Errorf("transcribe caption source: %w", err)
		}

		srt, chunks, duration, err := GenerateSRTFromSegments(transcript.Segments)
		if err == nil {
			if err := writeSRT(outputPath, srt); err != nil {
				return Result{}, err
			}
			logger.Info("generated captions from transcription", "source_file", audioPath, "output_file", outputPath, "chunks", chunks, "duration", duration.String())
			return Result{Path: outputPath, Status: StatusGenerated, Chunks: chunks, Duration: duration, Source: "transcription"}, nil
		}
		logger.Warn("transcription did not include usable timestamp segments; falling back to script timing", "error", err)

		if strings.TrimSpace(script) == "" {
			script = transcript.Text
		}
	}

	if strings.TrimSpace(script) == "" {
		return Result{}, errors.New("script.txt fallback is empty and transcription did not return timestamp segments")
	}
	if !isMP3Path(audioPath) {
		return Result{}, errors.New("transcription did not return timestamp segments and script fallback timing requires voice.mp3")
	}

	duration, err := MP3Duration(audioPath)
	if err != nil {
		return Result{}, fmt.Errorf("estimate voice.mp3 duration: %w", err)
	}

	srt, chunks, err := GenerateSRT(script, duration)
	if err != nil {
		return Result{}, err
	}

	if err := writeSRT(outputPath, srt); err != nil {
		return Result{}, err
	}

	logger.Info("generated captions from script fallback", "output_file", outputPath, "chunks", chunks, "duration", duration.String())
	return Result{Path: outputPath, Status: StatusGenerated, Chunks: chunks, Duration: duration, Source: "script_fallback"}, nil
}

func isMP3Path(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".mpeg", ".mpga":
		return true
	default:
		return false
	}
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

func GenerateSRTFromSegments(segments []Segment) (string, int, time.Duration, error) {
	captions := CaptionsFromSegments(segments)
	if len(captions) == 0 {
		return "", 0, 0, errors.New("transcription did not return timestamp segments")
	}

	var b strings.Builder
	for i, caption := range captions {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, formatTimestamp(caption.Start), formatTimestamp(caption.End), caption.Text)
	}
	return b.String(), len(captions), captions[len(captions)-1].End, nil
}

func CaptionsFromSegments(segments []Segment) []Caption {
	var captions []Caption
	for _, segment := range segments {
		segment.Text = normalizeWhitespace(segment.Text)
		if segment.Text == "" || segment.End <= segment.Start {
			continue
		}
		captions = append(captions, splitSegment(segment)...)
	}
	return captions
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

func splitSegment(segment Segment) []Caption {
	chunks := SplitScript(segment.Text)
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) == 1 {
		return []Caption{{Text: chunks[0], Start: segment.Start, End: segment.End}}
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

	captions := make([]Caption, 0, len(chunks))
	start := segment.Start
	var targetEnd float64
	segmentDuration := float64(segment.End - segment.Start)
	for i, chunk := range chunks {
		if i == len(chunks)-1 {
			targetEnd = segmentDuration
		} else {
			targetEnd += segmentDuration * float64(weights[i]) / float64(totalWeight)
		}
		end := segment.Start + time.Duration(math.Round(targetEnd))
		if end <= start {
			end = start + time.Millisecond
		}
		captions = append(captions, Caption{Text: chunk, Start: start, End: end})
		start = end
	}
	return captions
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

func readOptionalScript(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("script.txt path is empty")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("script.txt is missing at %s", path)
		}
		return "", fmt.Errorf("read script.txt: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

func writeSRT(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create captions output directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write captions.srt: %w", err)
	}
	return nil
}

func transcriberFromConfig(config Config, logger *slog.Logger) Transcriber {
	if config.Transcriber != nil {
		return config.Transcriber
	}
	if strings.TrimSpace(config.OpenAIAPIKey) == "" {
		return nil
	}
	return NewOpenAITranscriber(config.OpenAIAPIKey, config.OpenAITranscriptionModel, logger)
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(math.Round(seconds * float64(time.Second)))
}

func valueOrDefault(value string, fallback string) string {
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
