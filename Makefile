build-linux:
	GOOS=linux GOARCH=arm64 go build -o app-linux-arm64 ./cmd/radar

build-mac:
	GOOS=darwin GOARCH=arm64 go build -o app-mac-arm64 ./cmd/radar

build-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o app-mac-amd64 ./cmd/radar

build-local:
	go build -o app-local ./cmd/radar

# Lidar binary build targets
build-lidar-linux:
	GOOS=linux GOARCH=arm64 go build -o lidar-linux-arm64 ./cmd/lidar

build-lidar-mac:
	GOOS=darwin GOARCH=arm64 go build -o lidar-mac-arm64 ./cmd/lidar

build-lidar-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o lidar-mac-amd64 ./cmd/lidar

build-lidar-local:
	go build -o lidar-local ./cmd/lidar
