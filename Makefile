NAME := tfstatesubst
SRCS := $(shell find . -type f -name '*.go' -not -name '*_test.go')
REVISION := $(shell git rev-parse --short HEAD)
BUILD_FLAGS := -trimpath -ldflags "-s -X github.com/abicky/tfstatesubst/cmd.revision=$(REVISION)"

all: bin/$(NAME)

bin/$(NAME): $(SRCS)
	mkdir -p $(@D)
	go build -o $@ $(BUILD_FLAGS)

.PHONY: clean
clean:
	rm -f bin/$(NAME)

.PHONY: install
install:
	go install $(BUILD_FLAGS)

.PHONY: test
test:
	go test -v ./...

.PHONY: vet
vet:
	go vet ./...
