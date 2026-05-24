# History Shorts AI

Generate research, scripts, image prompts, titles, and descriptions for history-focused YouTube Shorts.

## Setup

Add your API key to `.env`:

```env
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-5.4-mini
OPENAI_TTS_MODEL=gpt-4o-mini-tts
OPENAI_TTS_VOICE=marin
OPENAI_TRANSCRIPTION_MODEL=gpt-4o-mini-transcribe
OPENAI_IMAGE_MODEL=gpt-image-1
OPENAI_IMAGE_SIZE=1024x1536
OPENAI_IMAGE_QUALITY=low
```

`OPENAI_MODEL`, `OPENAI_TTS_MODEL`, `OPENAI_TTS_VOICE`, `OPENAI_TRANSCRIPTION_MODEL`, `OPENAI_IMAGE_MODEL`, `OPENAI_IMAGE_SIZE`, and `OPENAI_IMAGE_QUALITY` are optional. The CLI defaults to `gpt-5.4-mini` for text generation, `gpt-4o-mini-tts` with the `marin` voice for TTS, `gpt-4o-mini-transcribe` for synced captions, and `gpt-image-1` at `1024x1536` with `low` quality for images.

Video rendering requires `ffmpeg` and `ffprobe` to be installed and available on `PATH`.

## Generate

```bash
go run cmd/generate/main.go \
  --topic "Why Did Alexander the Great Die at Just 32?" \
  --voice \
  --images \
  --captions \
  --render
```

The generator writes:

```text
output/alexander-the-great/
|-- research.txt
|-- script.txt
|-- image_prompts.json
|-- titles.txt
|-- description.txt
|-- images/
|   |-- 01.png
|   |-- 02.png
|   `-- 03.png
|-- voice.mp3
|-- captions.srt
`-- final.mp4
```

Generation order:

```text
TOPIC
  |
Research
  |
Script
  |
  |-- Image Prompts
  |-- Titles
  `-- Description
```

By default, rerunning the same topic reuses existing output files and only generates missing files. Pass `--force` to regenerate and overwrite existing files.

`--captions` creates `captions.srt` by transcribing `voice.mp3` and using the returned timestamp segments. `script.txt` is used only as a transcription prompt/fallback source if timestamp segments are unavailable. If `captions.srt` already exists, it is reused unless `--force` is passed; run captions with `--force` whenever `voice.mp3` changes.

`--render` creates `final.mp4` from `images/*.png`, `voice.mp3`, and `captions.srt` using FFmpeg. It renders a 1080x1920, 30 fps H.264/AAC MP4, normalizes the voiceover volume, and reuses an existing `final.mp4` unless `--force` is passed. If `voice.mp3` changes, regenerate captions and the video with `--force` so `captions.srt` uses the current audio timing.

Optional flags:

```bash
go run cmd/generate/main.go \
  --topic "The shortest war in history" \
  --model gpt-5.4-mini \
  --prompts prompts \
  --output output \
  --voice \
  --images \
  --captions \
  --render \
  --force
```

## Make Commands

```bash
make generate TOPIC="Why Did Alexander the Great Die at Just 32?"
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" VOICE=1
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" IMAGES=1
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" CAPTIONS=1
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" RENDER=1
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" FORCE=1
make test
make fmt
make tidy
```
