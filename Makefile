GO ?= go
TOPIC ?= Why Did Alexander the Great Die at Just 32?
MODEL ?=
PROMPTS ?= prompts
OUTPUT ?= output
VOICE ?=
IMAGES ?=
CAPTIONS ?=
FORCE ?=

GENERATE_ARGS := --topic "$(TOPIC)" --prompts "$(PROMPTS)" --output "$(OUTPUT)"
ifneq ($(strip $(MODEL)),)
GENERATE_ARGS += --model "$(MODEL)"
endif
ifneq ($(strip $(VOICE)),)
GENERATE_ARGS += --voice
endif
ifneq ($(strip $(IMAGES)),)
GENERATE_ARGS += --images
endif
ifneq ($(strip $(CAPTIONS)),)
GENERATE_ARGS += --captions
endif
ifneq ($(strip $(FORCE)),)
GENERATE_ARGS += --force
endif

.PHONY: help generate test fmt tidy

help:
	@echo "Available commands:"
	@echo "  make generate TOPIC=\"Why Did Alexander the Great Die at Just 32?\""
	@echo "  make generate TOPIC=\"Why Did Alexander the Great Die at Just 32?\" VOICE=1"
	@echo "  make generate TOPIC=\"Why Did Alexander the Great Die at Just 32?\" IMAGES=1"
	@echo "  make generate TOPIC=\"Why Did Alexander the Great Die at Just 32?\" CAPTIONS=1"
	@echo "  make generate TOPIC=\"Why Did Alexander the Great Die at Just 32?\" FORCE=1"
	@echo "  make test"
	@echo "  make fmt"
	@echo "  make tidy"

generate:
	$(GO) run cmd/generate/main.go $(GENERATE_ARGS)

test:
	$(GO) test ./...

fmt:
	gofmt -w cmd internal

tidy:
	$(GO) mod tidy
