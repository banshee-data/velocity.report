# velocity.report

A privacy-focused traffic logging tool for neighborhood change-makers. Measure vehicle speeds, make streets safer.

[![join-us-on-discord](https://github.com/user-attachments/assets/fa329256-aee7-4751-b3c4-d35bdf9287f5)](https://discord.gg/XXh6jXVFkt)

```
                                                ░░░░                            
                                               ▒▓███▓▓▓▓▒                       
                                                      ▒▓▒▒                      
                    ░▓▓▓▓▓▓▓▓▓▓▓▓░                    ░▓▒▒                      
                    ▒▓▓▓▓▓██████▓▓░                ▒▓██▓▒                       
                      ▒▒▓▒▓▓░                      ▒▓▒░                         
                         ░▓▓░                       ▓▒▒                         
                          ░▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓██████▓░                        
                          ▓▓█▓▒▒▒▒▒▒░░░░            ░▓▓▒                        
                        ░▓▓▒▓▓░                   ░▒▓▓▓▓░                       
           ░░▒▒▒▒░     ░▓▒░ ▒▓▒                  ▒▓▓▒ ▓▓▒ ░▒▒▓▓▒▒░              
        ▒▓▓▓██▓▓▓██▓▓▓▓▓▒   ░▓▓░               ▒▓▓▒   ▒██▓▓█▓▓▓▓▓▓▒▓▓▒░         
     ░▓▓▓▓▓▒░ ░    ▒▒██▓▒▒   ▒▓▒░            ░▓▓▒   ▓▓█▓█▓    ░░   ▒▒▓▒▓▒       
    ▒▓▓▓▓░    ░░   ░▓▓░▒▓▓▒   ▒▓▒           ▓▓▓   ░▓▓▓░░▓▓░   ░░      ▒█▓▒▒     
  ░▒█▒▓░▒     ░░  ░▓▒░  ░▓▓▒░ ░▓▓ ░████▓  ▒█▓░   ▒▓█▒   ░▓▓░  ░░     ▒░░▒█▓▒    
  ▒▓▒▓   ▒░   ░░ ▒▓▓     ░▓▓▓░ ░▓▒ ░▓▒  ░▓▓▒    ▒▓█▒░▒   ▒█▒  ░    ▒░   ░▒▓▓▒   
 ░▓█▓     ░▒░ ░░▒▓▒       ░▓▓▒░▒▓████▒ ▒▓▓░    ▒▓▓▒    ▒▒ ▓▓▒ ░  ░▒      ░▓▓▒░  
 ▒▓▓▒       ░▒▒▓█▓▓▓███▓▓▓▓████▓█▓▓▒▒▓▓▓▒      ▒█▓░      ░▓▓▓▒▒▒▒         ▒▓▒▒  
 ▒▓▓▒░░▒▒▒▓▒▒▓▓██▓▒▒░▒░░░  ▒▓▒▒▓▓▓▓▓█▓▓▓░      ▒█▓░      ░░░▒█▓▓▒▒▒░░░░░░░▒█▓░  
 ▒▓▓▒       ░▒▒▓▒░         ▓▓▓░▒▓▓▓▓▓▒▒▓░      ▒▓▓▒▒░░░░░   ░▒▓░▒░        ▓▓▓░  
 ░▓█▓░    ░▒░ ░▒  ▒░      ▒▓▓▒  ▒███▓▓▓░       ░▓▓▒       ░░  ░  ░▒░     ░▓▓▓░  
  ▒▓▓▓░  ▒░    ▒    ▒░   ░▓▓▓░    ▒▓            ▓▓▓░     ▒    ░░   ░▒░  ░▓█▓░   
   ▒▓▓▓▒░     ░▒     ░▒ ▒▓▓▒     ░▓▒            ░▒▓▓░  ░░     ░░      ▒▒▓▓▓░    
    ▒▓▓▓▒░     ▒      ░▓▓▓▒     ▓█▓▓█░            ▒▓▓▓▒░      ▒░     ░▓█▓▓░     
     ░▒▓██▓▒▒  ▒  ░▒▓▓█▓▒░                         ░▓▓█▓▓░    ░░ ░▒▓█▓▓▓░       
      ░░░▒▒▓▓████▓██▓▓░                ░░             ▒▓▓▓▓██▓▓▓████▓▒░         
  ░░░░░░░░░░░░▒▒░░░░░░░░░░░░░░░░░░░░░ ░░░░░░░░░░░░░░░░ ░░░▒▒▒▒▓▒▒░░░░░░░░░░     
      ░░░░░░░░░░░░░░░░░░░░░░ ░░░░ ░░░░░░░░░░  ░░░░░░░░░░░░░░░░░░░░░ ░░░░░░░░░   
   ░░░ ░░░░░░   ░░░░ ░░░░░░░░░░░░ ░░░░░   ░░░░░░   ░░░░░░ ░░░░░░░░░░░ ░░░░      
     ░░░    ░░░░   ░░░░ ░░░░    ░░░    ░░░░    ░░░░░   ░░░░░   ░░░░░            
```


## Develop

Build and run the development server with the following command. If an existing SQLite database is available, place it in `.build/sensor_data.db`

```sh
make build-local
cd .build
./app-local -dev
```

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