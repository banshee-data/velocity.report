radar-linux:
	GOOS=linux GOARCH=arm64 go build -o app-radar-linux-arm64 ./cmd/radar

radar-mac:
	GOOS=darwin GOARCH=arm64 go build -o app-radar-mac-arm64 ./cmd/radar

radar-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o app-radar-mac-amd64 ./cmd/radar

radar-local:
	go build -o app-radar-local ./cmd/radar

# Lidar binary build targets
lidar-linux:
	GOOS=linux GOARCH=arm64 go build -o app-lidar-linux-arm64 ./cmd/lidar

lidar-mac:
	GOOS=darwin GOARCH=arm64 go build -o app-lidar-mac-arm64 ./cmd/lidar

lidar-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o app-lidar-mac-amd64 ./cmd/lidar

lidar-local:
	go build -o app-lidar-local ./cmd/lidar
