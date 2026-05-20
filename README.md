# History Shorts AI

Generate research, scripts, image prompts, titles, and descriptions for history-focused YouTube Shorts.

## Setup

Add your API key to `.env`:

```env
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-5.2
```

`OPENAI_MODEL` is optional. The CLI defaults to `gpt-5.2`.

## Generate

```bash
go run cmd/generate/main.go \
  --topic "Why Did Alexander the Great Die at Just 32?"
```

The generator writes:

```text
output/alexander-the-great/
|-- research.txt
|-- script.txt
|-- image_prompts.json
|-- titles.txt
`-- description.txt
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
  --model gpt-5.2 \
  --prompts prompts \
  --output output
```
