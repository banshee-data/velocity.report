# Velocity.Report by Banshee

## Build 

Use the following to build the go binary for the Raspberry Pi:

```sh
GOARCH=arm64 GOOS=linux go build -o app .
```


## Deploy 

Stop service:

```sh
sudo systemctl stop go-sensor.service
```

Copy app

Start service

```sh
sudo systemctl start go-sensor.service
```

Monitor logs:

```sh
sudo journalctl -u go-sensor.service -f
```