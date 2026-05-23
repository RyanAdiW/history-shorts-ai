# History Shorts AI

Generate research, scripts, image prompts, titles, and descriptions for history-focused YouTube Shorts.

## Setup

Add your API key to `.env`:

```env
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-5.4-mini
OPENAI_TTS_MODEL=gpt-4o-mini-tts
OPENAI_TTS_VOICE=marin
OPENAI_IMAGE_MODEL=gpt-image-1
OPENAI_IMAGE_SIZE=1024x1536
```

`OPENAI_MODEL`, `OPENAI_TTS_MODEL`, `OPENAI_TTS_VOICE`, `OPENAI_IMAGE_MODEL`, and `OPENAI_IMAGE_SIZE` are optional. The CLI defaults to `gpt-5.4-mini` for text generation, `gpt-4o-mini-tts` with the `marin` voice for TTS, and `gpt-image-1` at `1024x1536` for images.

## Generate

```bash
go run cmd/generate/main.go \
  --topic "Why Did Alexander the Great Die at Just 32?" \
  --voice \
  --images
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
`-- voice.mp3
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

Optional flags:

```bash
go run cmd/generate/main.go \
  --topic "The shortest war in history" \
  --model gpt-5.4-mini \
  --prompts prompts \
  --output output \
  --voice \
  --images \
  --force
```

## Make Commands

```bash
make generate TOPIC="Why Did Alexander the Great Die at Just 32?"
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" VOICE=1
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" IMAGES=1
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" FORCE=1
make test
make fmt
make tidy
```
