package caption

import (
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

func TestGenerateFromFilesReusesExistingCaptions(t *testing.T) {
	dir := t.TempDir()
	captionsPath := filepath.Join(dir, "captions.srt")
	if err := os.WriteFile(captionsPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GenerateFromFiles(Config{OutputPath: captionsPath})
	if err != nil {
		t.Fatalf("GenerateFromFiles returned error: %v", err)
	}
	if result.Status != StatusReused {
		t.Fatalf("status = %q, want %q", result.Status, StatusReused)
	}
}

func TestGenerateFromFilesMissingScript(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateFromFiles(Config{
		ScriptPath: filepath.Join(dir, "script.txt"),
		AudioPath:  filepath.Join(dir, "voice.mp3"),
		OutputPath: filepath.Join(dir, "captions.srt"),
		Force:      true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "script.txt is missing")
}

func TestGenerateFromFilesEmptyScript(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "script.txt")
	if err := os.WriteFile(scriptPath, []byte(" \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := GenerateFromFiles(Config{
		ScriptPath: scriptPath,
		AudioPath:  filepath.Join(dir, "voice.mp3"),
		OutputPath: filepath.Join(dir, "captions.srt"),
		Force:      true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "script.txt is empty")
}

func TestGenerateFromFilesMissingVoice(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "script.txt")
	if err := os.WriteFile(scriptPath, []byte("Alexander crossed into Asia."), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := GenerateFromFiles(Config{
		ScriptPath: scriptPath,
		AudioPath:  filepath.Join(dir, "voice.mp3"),
		OutputPath: filepath.Join(dir, "captions.srt"),
		Force:      true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "voice.mp3 is missing")
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
