# tcp-proxy
[![go-tcp-proxy CI](https://github.com/lutfailham96/go-http-injector/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/lutfailham96/go-http-injector/actions/workflows/ci.yml)
[![Maintainability](https://api.codeclimate.com/v1/badges/b458531c1805fb033210/maintainability)](https://codeclimate.com/github/lutfailham96/go-http-injector/maintainability)

A small TCP proxy written in Go

This project was intended for debugging text-based protocols. The next version will address binary protocols.

## Install

**Source**

``` sh
$ go get -v github.com/lutfailham96/go-http-injector/cmd/tcp-proxy
```

## Usage

```
$ tcp-proxy --help
Usage of tcp-proxy:
  -c: output ansi colors
  -h: output hex
  -l="localhost:9999": local address
  -n: disable nagles algorithm
  -r="localhost:80": remote address
  -s="server address / sni address": server:443
  -rp="use as reverse proxy"
  -in-payload="payload to be used as incoming TCP packet"
  -out-payload="payload to be used as outbound TCP packet"
  -v: display server actions
  -vv: display server actions and all tcp data
```

*Note: Regex match and replace*
**only works on text strings**
*and does NOT work across packet boundaries*

### Client Example

Use custom payload
```shell
$ go-tcp-proxy \
    -l 127.0.0.1:9999 \
    -r 127.0.0.1:10443 \
    -s myserver:443 \
    -out-payload="GET ws://cloudflare.com HTTP/1.1[crlf]Host: [host][crlf]Upgrade: websocket[crlf]Connection: keep-alive[crlf][crlf]"

Proxying from 127.0.0.1:9999 to 104.15.50.1:443
```

stunnel configuration
```
[ws]
client = yes
accept = 127.0.0.1:10443
connect = 104.15.50.5:443
sni = cloudflare.com
cert = /etc/stunnel/ssl/stunnel.pem

```

Tunnel over SSH conneciton
```shell
$ ssh -o "ProxyCommand=corkscrew 127.0.0.1 9999 %h %p" -v4ND 1080 my-user@localhost
```

### Todo

* Implement `tcpproxy.Conn` which provides accounting and hooks into the underlying `net.Conn`
* Verify wire protocols by providing `encoding.BinaryUnmarshaler` to a `tcpproxy.Conn`
* Modify wire protocols by also providing a map function
* Implement [SOCKS v5](https://www.ietf.org/rfc/rfc1928.txt) to allow for user-decided remote addresses
