package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindImagesSortsPNGs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "02.png"), "png")
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "ignore")
	writeTestFile(t, filepath.Join(dir, "01.PNG"), "png")

	images, err := findImages(dir)
	if err != nil {
		t.Fatalf("findImages returned error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("images length = %d, want 2", len(images))
	}
	if filepath.Base(images[0]) != "01.PNG" || filepath.Base(images[1]) != "02.png" {
		t.Fatalf("images not sorted: %#v", images)
	}
}

func TestValidateInputsMissingImagesDir(t *testing.T) {
	dir := t.TempDir()
	_, err := validateRawInputs(Config{
		ImagesDir:     filepath.Join(dir, "images"),
		AudioPath:     filepath.Join(dir, "voice.mp3"),
		RawOutputPath: filepath.Join(dir, "raw.mp4"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "images directory is missing")
}

func TestValidateInputsMissingVoice(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(imagesDir, "01.png"), "png")

	_, err := validateRawInputs(Config{
		ImagesDir:     imagesDir,
		AudioPath:     filepath.Join(dir, "voice.mp3"),
		RawOutputPath: filepath.Join(dir, "raw.mp4"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "voice.mp3 is missing")
}

func TestValidateFinalInputsMissingRaw(t *testing.T) {
	dir := t.TempDir()

	_, err := validateFinalInputs(Config{
		RawOutputPath: filepath.Join(dir, "raw.mp4"),
		CaptionsPath:  filepath.Join(dir, "captions.srt"),
		OutputPath:    filepath.Join(dir, "final.mp4"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "raw.mp4 is missing")
}

func TestValidateFinalInputsMissingCaptions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "raw.mp4"), "mp4")

	_, err := validateFinalInputs(Config{
		RawOutputPath: filepath.Join(dir, "raw.mp4"),
		CaptionsPath:  filepath.Join(dir, "captions.srt"),
		OutputPath:    filepath.Join(dir, "final.mp4"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "captions.srt is missing")
}

func TestWriteConcatFile(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	imageOne := filepath.Join(imagesDir, "01.png")
	imageTwo := filepath.Join(imagesDir, "02.png")
	writeTestFile(t, imageOne, "png")
	writeTestFile(t, imageTwo, "png")

	concatPath, err := writeConcatFile(dir, []string{imageOne, imageTwo})
	if err != nil {
		t.Fatalf("writeConcatFile returned error: %v", err)
	}
	defer os.Remove(concatPath)

	content, err := os.ReadFile(concatPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	assertContains(t, got, "file 'images/01.png'\n")
	assertContains(t, got, "file 'images/02.png'\n")
}

func TestBuildRawFFmpegArgs(t *testing.T) {
	args := strings.Join(buildRawFFmpegArgs("concat.txt", "voice.mp3", "raw.mp4", 45, 1500*time.Millisecond, 2.5), " ")
	assertContains(t, args, "-c:v libx264")
	assertContains(t, args, "-c:a aac")
	assertContains(t, args, "-af volume=2.5")
	assertContains(t, args, "-map 0:v:0 -map 1:a:0")
	assertContains(t, args, "-t 1.500")
	assertContains(t, args, "scale=1080:1920")
	assertContains(t, args, "zoompan=")
	assertContains(t, args, ":d=45:")
	assertNotContains(t, args, "subtitles=")
	assertContains(t, args, "raw.mp4")
}

func TestBuildFinalFFmpegArgs(t *testing.T) {
	args := strings.Join(buildFinalFFmpegArgs("raw.mp4", "captions.srt", "final.mp4"), " ")
	assertContains(t, args, "-c:v libx264")
	assertContains(t, args, "-c:a aac")
	assertContains(t, args, "-map 0:v:0 -map 0:a:0")
	assertContains(t, args, "subtitles=captions.srt")
	assertContains(t, args, "raw.mp4")
	assertContains(t, args, "final.mp4")
}

func TestParseVoiceVolume(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "empty default", input: "", want: DefaultVoiceVolume},
		{name: "valid", input: "2.5", want: 2.5},
		{name: "trims", input: " 1.25 ", want: 1.25},
		{name: "zero invalid", input: "0", wantErr: true},
		{name: "negative invalid", input: "-1", wantErr: true},
		{name: "text invalid", input: "loud", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseVoiceVolume(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("volume = %v, want %v", got, test.want)
			}
		})
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, value string, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("%q does not contain %q", value, substring)
	}
}

func assertNotContains(t *testing.T, value string, substring string) {
	t.Helper()
	if strings.Contains(value, substring) {
		t.Fatalf("%q contains %q", value, substring)
	}
}
