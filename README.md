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
VIDEO_VOICE_VOLUME=2.0
```

`OPENAI_MODEL`, `OPENAI_TTS_MODEL`, `OPENAI_TTS_VOICE`, `OPENAI_TRANSCRIPTION_MODEL`, `OPENAI_IMAGE_MODEL`, `OPENAI_IMAGE_SIZE`, `OPENAI_IMAGE_QUALITY`, and `VIDEO_VOICE_VOLUME` are optional. The CLI defaults to `gpt-5.4-mini` for text generation, `gpt-4o-mini-tts` with the `marin` voice for TTS, `gpt-4o-mini-transcribe` for synced captions, `gpt-image-1` at `1024x1536` with `low` quality for images, and `2.0` for rendered voice volume.

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
|-- raw.mp4
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

`--captions` creates `captions.srt` by transcribing `raw.mp4` when it exists, falling back to `voice.mp3` when it does not. The captions use returned transcription timestamp segments; `script.txt` is used only as a transcription prompt/fallback source if timestamp segments are unavailable. If `captions.srt` already exists, it is reused unless `--force` is passed.

`--render` uses a two-pass FFmpeg flow. It first creates `raw.mp4` from `images/*.png` and `voice.mp3` with no burned captions, boosts the voiceover volume with `VIDEO_VOICE_VOLUME`, and reuses an existing `raw.mp4` unless `--force` is passed. If `captions.srt` exists, it then burns those captions into `raw.mp4` to create `final.mp4`; if captions are missing, it leaves `raw.mp4` in place and logs that final rendering was skipped.

For synced captions, run render once to produce `raw.mp4`, run captions with `--force`, then render again:

```bash
go run cmd/generate/main.go --topic "The Shortest War in History" --render
go run cmd/generate/main.go --topic "The Shortest War in History" --captions --force
go run cmd/generate/main.go --topic "The Shortest War in History" --render --force
```

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
