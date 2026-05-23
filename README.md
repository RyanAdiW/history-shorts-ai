# History Shorts AI

Generate research, scripts, image prompts, titles, and descriptions for history-focused YouTube Shorts.

## Setup

Add your API key to `.env`:

```env
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-5.4-mini
OPENAI_TTS_MODEL=gpt-4o-mini-tts
OPENAI_TTS_VOICE=marin
```

`OPENAI_MODEL`, `OPENAI_TTS_MODEL`, and `OPENAI_TTS_VOICE` are optional. The CLI defaults to `gpt-5.4-mini` for text generation and `gpt-4o-mini-tts` with the `marin` voice for TTS.

## Generate

```bash
go run cmd/generate/main.go \
  --topic "Why Did Alexander the Great Die at Just 32?" \
  --voice
```

The generator writes:

```text
output/alexander-the-great/
|-- research.txt
|-- script.txt
|-- image_prompts.json
|-- titles.txt
|-- description.txt
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

Optional flags:

```bash
go run cmd/generate/main.go \
  --topic "The shortest war in history" \
  --model gpt-5.4-mini \
  --prompts prompts \
  --output output \
  --voice
```

## Make Commands

```bash
make generate TOPIC="Why Did Alexander the Great Die at Just 32?"
make generate TOPIC="Why Did Alexander the Great Die at Just 32?" VOICE=1
make test
make fmt
make tidy
```
