BINARY := ig-dl
PKG    := github.com/ibrahimhajjaj/ig-dl

.PHONY: build install test test-integration vet staticcheck lint smoke clean

build:
	go build -o $(BINARY) ./cmd/ig-dl

install:
	go install ./cmd/ig-dl

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

vet:
	go vet ./...

staticcheck:
	@which staticcheck > /dev/null || (echo "install: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)
	staticcheck ./...

lint: vet staticcheck

smoke: build
	IG_DL_BIN=$$PWD/$(BINARY) ./scripts/smoke.sh

clean:
	rm -f $(BINARY)
	rm -rf dist/ downloads/
