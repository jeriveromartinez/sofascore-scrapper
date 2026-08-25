GO_MODULE := github.com/jeriveromartinez/sofascore-scrapper
PROTO_FILE := proto/api.proto
GENERATED_DIR := internal/gen
FLUTTER_PROTO_URL := https://raw.githubusercontent.com/jeriveromartinez/flutter-apptv/main/proto/api.proto

.PHONY: proto proto-check proto-verify-flutter help

help:
	@echo "Available targets:"
	@echo "  make proto                  Regenerate Go bindings from $(PROTO_FILE)"
	@echo "  make proto-check           Regenerate and fail if internal/gen/ has uncommitted changes"
	@echo "  make proto-verify-flutter   Fail if flutter-apptv's proto/api.proto drifts from this one"

# Regenerate Go bindings from proto/api.proto.
# Output lands in $(GENERATED_DIR) because that matches the `option go_package`
# in $(PROTO_FILE).
proto:
	protoc \
		--go_out=. \
		--go_opt=module=$(GO_MODULE) \
		$(PROTO_FILE)

# Regenerate and fail if the working tree has uncommitted changes inside
# $(GENERATED_DIR). Used by the CI `proto-diff` job to catch stale bindings.
proto-check: proto
	@if ! git diff --exit-code $(GENERATED_DIR) > /dev/null 2>&1; then \
		echo "Generated protobuf code is out of date. Run 'make proto' and commit."; \
		git diff $(GENERATED_DIR); \
		exit 1; \
	fi
	@echo "Generated protobuf code is up to date."

# Verify the Flutter client's proto/api.proto is a verbatim copy of this repo's.
# The contract is documented in docs/integration-audit-roadmap.md: the Go repo
# is the source of truth and flutter-apptv must mirror proto/api.proto.
proto-verify-flutter:
	@curl -fsSL $(FLUTTER_PROTO_URL) -o /tmp/flutter-api.proto
	@if ! diff -q $(PROTO_FILE) /tmp/flutter-api.proto > /dev/null 2>&1; then \
		echo "Flutter proto is out of sync with the Go source of truth."; \
		echo "Run 'cp $(PROTO_FILE) ../flutter-apptv/proto/api.proto' and regenerate the Dart bindings."; \
		diff $(PROTO_FILE) /tmp/flutter-api.proto | head -80; \
		exit 1; \
	fi
	@echo "Flutter proto is in sync."
