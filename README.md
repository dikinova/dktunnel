[![Build Status](https://travis-ci.org/dikinova/dktunnel.svg?branch=master)](https://travis-ci.org/dikinova/dktunnel)

## dktunnel
dktunnel is a secure tcp tunnel software. It can use tcp or udp connectioin as low level tunnel.

dktunnel could be added to any c/s system using tcp protocol. Make system structure evolve from
```
client <--------------> server
```
to
```
client <-> dktunnel <--------------> dktunnel <-> server
```
to gain dktunnel's valuable features, such as secure and persistent. 

## build

```bash
go install github.com/dikinova/dktunnel
```


## Usage

```
usage: bin/dktunnel
  -c
        client
  -s
        server
  -listen string
        listen address (default ":8001")
  -backend string
        backend address (default "127.0.0.1:1234")
  -secret string
        tunnel secret (default "the answer to life, the universe and everything")
  -log uint
        log level (default 1)

```

some options:
* secret: for authentication and exchanging encryption key
* available ciphers: AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20IETF CHACHA20X RC4-128 RC4-256


## Example
Suppose you have a squid server, and you use it as a http proxy. Usually, you will start the server:
```
$ squid3 -a 8080
```
and use it on your pc:
```
curl --proxy server:8080 http://example.com
```
It works fine but all traffic between your server and pc is plaintext, so someone can monitor you easily. In this case, dktunnel could help to encrypt your traffic.

First, on your server, resart squid to listen on a local port, for example **127.0.0.1:3128**. Then start dktunnel server listen on 8080 and use **127.0.0.1:3128** as backend.
```
$ ./dktunnel -s -listen=:8001 -backend=127.0.0.1:3128 -secret="your secret"
```
Second, on your pc, start dktunnel client:
```
$ ./dktunnel -c -listen="127.0.0.1:8080" -backend="server:8001" -secret="your secret"
```

Then you can use squid3 on you local port as before, but all your traffic is encrypted. 

Besides that, you don't need to create and destory tcp connection between your pc and server, because dktunnel use long-live tcp connections as low tunnel. In most cases, it would be faster.

## licence
The MIT License (MIT)

Copyright (c) 2018 dikinova

