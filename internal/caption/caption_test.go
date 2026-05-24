package caption

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplitScriptKeepsChunksReadable(t *testing.T) {
	script := "Alexander ruled fast and fought hard. His empire stretched across continents, but fever and politics closed in quickly after the final campaign."

	chunks := SplitScript(script)
	if len(chunks) < 3 {
		t.Fatalf("expected several chunks, got %d: %#v", len(chunks), chunks)
	}

	for _, chunk := range chunks {
		wordCount := len(strings.Fields(chunk))
		if wordCount > maxWordsPerCaption {
			t.Fatalf("chunk has %d words, want at most %d: %q", wordCount, maxWordsPerCaption, chunk)
		}
	}
}

func TestGenerateSRT(t *testing.T) {
	srt, chunks, err := GenerateSRT("One two three four five six seven eight nine ten.", 5*time.Second)
	if err != nil {
		t.Fatalf("GenerateSRT returned error: %v", err)
	}
	if chunks != 2 {
		t.Fatalf("chunks = %d, want 2", chunks)
	}
	assertContains(t, srt, "1\n00:00:00,000 -->")
	assertContains(t, srt, "\n2\n")
	assertContains(t, srt, "--> 00:00:05,000\n")
}

func TestGenerateSRTFromSegmentsSplitsLongSegments(t *testing.T) {
	srt, chunks, duration, err := GenerateSRTFromSegments([]Segment{
		{
			Text:  "One two three four five six seven eight nine ten eleven twelve.",
			Start: time.Second,
			End:   7 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("GenerateSRTFromSegments returned error: %v", err)
	}
	if chunks != 2 {
		t.Fatalf("chunks = %d, want 2", chunks)
	}
	if duration != 7*time.Second {
		t.Fatalf("duration = %s, want 7s", duration)
	}
	assertContains(t, srt, "1\n00:00:01,000 -->")
	assertContains(t, srt, "\n2\n")
	assertContains(t, srt, "--> 00:00:07,000\n")
}

func TestGenerateFromFilesReusesExistingCaptions(t *testing.T) {
	dir := t.TempDir()
	captionsPath := filepath.Join(dir, "captions.srt")
	if err := os.WriteFile(captionsPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GenerateFromFiles(context.Background(), Config{OutputPath: captionsPath})
	if err != nil {
		t.Fatalf("GenerateFromFiles returned error: %v", err)
	}
	if result.Status != StatusReused {
		t.Fatalf("status = %q, want %q", result.Status, StatusReused)
	}
}

func TestGenerateFromFilesUsesTranscriptionSegments(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "voice.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "script.txt")
	if err := os.WriteFile(scriptPath, []byte("fallback script"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GenerateFromFiles(context.Background(), Config{
		ScriptPath:  scriptPath,
		AudioPath:   audioPath,
		OutputPath:  filepath.Join(dir, "captions.srt"),
		Transcriber: mockTranscriber{transcript: Transcript{Segments: []Segment{{Text: "hello world", Start: time.Second, End: 2 * time.Second}}}},
		Force:       true,
	})
	if err != nil {
		t.Fatalf("GenerateFromFiles returned error: %v", err)
	}
	if result.Source != "transcription" {
		t.Fatalf("source = %q, want transcription", result.Source)
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(content), "00:00:01,000 --> 00:00:02,000")
	assertContains(t, string(content), "hello world")
}

func TestGenerateFromFilesFallsBackToScriptWithoutSegments(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "voice.mp3")
	if err := os.WriteFile(audioPath, testMP3Frame(), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "script.txt")
	if err := os.WriteFile(scriptPath, []byte("Alexander crossed into Asia quickly."), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GenerateFromFiles(context.Background(), Config{
		ScriptPath:  scriptPath,
		AudioPath:   audioPath,
		OutputPath:  filepath.Join(dir, "captions.srt"),
		Transcriber: mockTranscriber{transcript: Transcript{Text: "transcribed text only"}},
		Force:       true,
	})
	if err != nil {
		t.Fatalf("GenerateFromFiles returned error: %v", err)
	}
	if result.Source != "script_fallback" {
		t.Fatalf("source = %q, want script_fallback", result.Source)
	}
}

func TestGenerateFromFilesMissingVoice(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "script.txt")
	if err := os.WriteFile(scriptPath, []byte("Alexander crossed into Asia."), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := GenerateFromFiles(context.Background(), Config{
		ScriptPath:  scriptPath,
		AudioPath:   filepath.Join(dir, "voice.mp3"),
		OutputPath:  filepath.Join(dir, "captions.srt"),
		Transcriber: mockTranscriber{},
		Force:       true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "caption transcription source is missing")
}

func TestTranscriptionContentTypeSupportsVideo(t *testing.T) {
	if got := transcriptionContentType("raw.mp4"); got != "video/mp4" {
		t.Fatalf("content type = %q, want video/mp4", got)
	}
	if got := transcriptionContentType("voice.mp3"); got != "audio/mpeg" {
		t.Fatalf("content type = %q, want audio/mpeg", got)
	}
}

func TestMP3Duration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "voice.mp3")
	frame := testMP3Frame()
	data := make([]byte, 0, len(frame)*10)
	for i := 0; i < 10; i++ {
		data = append(data, frame...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	duration, err := MP3Duration(path)
	if err != nil {
		t.Fatalf("MP3Duration returned error: %v", err)
	}
	if duration < 250*time.Millisecond || duration > 270*time.Millisecond {
		t.Fatalf("duration = %s, want about 261ms", duration)
	}
}

func testMP3Frame() []byte {
	header := uint32(0xffe00000) | uint32(3)<<19 | uint32(1)<<17 | uint32(1)<<16 | uint32(9)<<12
	frame := make([]byte, 417)
	frame[0] = byte(header >> 24)
	frame[1] = byte(header >> 16)
	frame[2] = byte(header >> 8)
	frame[3] = byte(header)
	return frame
}

func assertContains(t *testing.T, value string, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("%q does not contain %q", value, substring)
	}
}

type mockTranscriber struct {
	transcript Transcript
	err        error
}

func (m mockTranscriber) Transcribe(context.Context, string, string) (Transcript, error) {
	return m.transcript, m.err
}
