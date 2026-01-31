APP_NAME = injector
BUILD_DIR = bin

.PHONY: all build install uninstall clean test lint

all: build

build:
	@echo "Building $(APP_NAME)..."
	go build -ldflags -o bin/$(APP_NAME) .

install:
	@echo "Installing $(APP_NAME)..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	mkdir -p "$$bin_dir"; \
	echo "Installing to $$bin_dir/$(APP_NAME)"; \
	go build -o "$$bin_dir/$(APP_NAME)" .

uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	echo "Removing $$bin_dir/$(APP_NAME)"; \
	rm -f "$$bin_dir/$(APP_NAME)"

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

test:
	go test ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		@echo "golangci-lint is not installed"; \
		exit 1; \
	}
	golangci-lint run
