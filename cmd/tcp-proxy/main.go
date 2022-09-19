package main

import (
	"flag"
	"fmt"
	proxy "github.com/lutfailham96/go-http-injector"
	"net"
	"os"
)

var (
	version = "0.0.0-src"
	connid  = uint64(0)

	localAddr       = flag.String("l", ":9999", "local address")
	remoteAddr      = flag.String("r", "localhost:80", "remote address")
	serverAddr      = flag.String("s", "", "server address")
	verbose         = flag.Bool("v", false, "display server actions")
	veryverbose     = flag.Bool("vv", false, "display server actions and all tcp data")
	nagles          = flag.Bool("n", false, "disable nagles algorithm")
	hex             = flag.Bool("h", false, "output hex")
	colors          = flag.Bool("c", false, "output ansi colors")
	unwrapTLS       = flag.Bool("unwrap-tls", false, "remote connection with TLS exposed unencrypted locally")
	outboundPayload = flag.String("out-payload", "", "outbound payload to be used as TCP request")
	incomingPayload = flag.String("in-payload", "HTTP/1.1 200 Connection Established", "incoming payload to be used as replacer of any error response")
)

func main() {
	flag.Parse()

	logger := proxy.ColorLogger{
		Verbose: *verbose,
		Color:   *colors,
	}

	logger.Info("go-tcp-proxy (%s) proxing from %v to %v ", version, *localAddr, *remoteAddr)

	laddr, err := net.ResolveTCPAddr("tcp", *localAddr)
	if err != nil {
		logger.Warn("Failed to resolve local address: %s", err)
		os.Exit(1)
	}
	raddr, err := net.ResolveTCPAddr("tcp", *remoteAddr)
	if err != nil {
		logger.Warn("Failed to resolve remote address: %s", err)
		os.Exit(1)
	}
	var saddr string
	if *serverAddr != "" {
		_, err := net.ResolveTCPAddr("tcp", *serverAddr)
		if err != nil {
			logger.Warn("Failed to resolve remote address: %s", err)
			os.Exit(1)
		}
		saddr = *serverAddr
	}
	listener, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		logger.Warn("Failed to open local port to listen: %s", err)
		os.Exit(1)
	}

	if *veryverbose {
		*verbose = true
	}

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			logger.Warn("Failed to accept connection '%s'", err)
			continue
		}
		connid++

		var p *proxy.Proxy
		if *unwrapTLS {
			logger.Info("Unwrapping TLS")
			p = proxy.NewTLSUnwrapped(conn, laddr, raddr, saddr, *remoteAddr)
		} else {
			p = proxy.New(conn, laddr, raddr, saddr)
		}

		p.SetOutboundConnPayload(*outboundPayload)
		p.SetIncomingConnPayload(*incomingPayload)

		p.Nagles = *nagles
		p.OutputHex = *hex
		p.Log = proxy.ColorLogger{
			Verbose:     *verbose,
			VeryVerbose: *veryverbose,
			Prefix:      fmt.Sprintf("Connection #%03d ", connid),
			Color:       *colors,
		}

		go p.Start()
	}
}
