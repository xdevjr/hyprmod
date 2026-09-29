WAILS3 ?= wails3
TAGS ?= gtk3

.PHONY: bindings frontend build test test-gui

bindings:
	$(WAILS3) generate bindings -ts -i -f '-tags $(TAGS)' -d frontend/bindings ./...

frontend:
	cd frontend && npm ci && npm run build

build: frontend
	mkdir -p bin
	go build -tags $(TAGS) -o bin/hyprmod .

test:
	go test ./...

test-gui:
	go test -tags $(TAGS) ./...
