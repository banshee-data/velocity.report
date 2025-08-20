build-linux:
	GOOS=linux GOARCH=arm64 go build -o app-linux-arm64 ./cmd/radar

build-mac:
	GOOS=darwin GOARCH=arm64 go build -o app-mac-arm64 ./cmd/radar

build-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o app-mac-amd64 ./cmd/radar

build-local:
	go build -o app-local ./cmd/radar
