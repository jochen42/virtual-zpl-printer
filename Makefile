BIN := bin/virtual-zpl-printer
TAGS := desktop,production

# Linux distributions shipping only webkit2gtk-4.1 (Ubuntu 24.04+) need the webkit2_41 tag.
ifeq ($(shell uname -s),Linux)
TAGS := $(TAGS),webkit2_41
endif

.PHONY: build run headless test dist clean

build:
	go build -tags $(TAGS) -ldflags "-w -s" -o $(BIN) .

run: build
	./$(BIN)

headless:
	go run . -headless

test:
	go vet ./... && go test ./...

# Release archive for the current platform in dist/ (CI builds all platforms on tag).
dist:
	scripts/package.sh $(shell go env GOOS) $(shell go env GOARCH) $(or $(VERSION),dev)

clean:
	rm -rf bin build dist
