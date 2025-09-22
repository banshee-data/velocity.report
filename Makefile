radar-linux:
	GOOS=linux GOARCH=arm64 go build -o app-radar-linux-arm64 ./cmd/radar

radar-mac:
	GOOS=darwin GOARCH=arm64 go build -o app-radar-mac-arm64 ./cmd/radar

radar-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o app-radar-mac-amd64 ./cmd/radar

radar-local:
	go build -o app-radar-local ./cmd/radar
