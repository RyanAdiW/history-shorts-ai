GO ?= go
TOPIC ?= Why Did Alexander the Great Die at Just 32?
MODEL ?=
PROMPTS ?= prompts
OUTPUT ?= output

GENERATE_ARGS := --topic "$(TOPIC)" --prompts "$(PROMPTS)" --output "$(OUTPUT)"
ifneq ($(strip $(MODEL)),)
GENERATE_ARGS += --model "$(MODEL)"
endif

.PHONY: help generate test fmt tidy

help:
	@echo "Available commands:"
	@echo "  make generate TOPIC=\"Why Did Alexander the Great Die at Just 32?\""
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
