package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	proxy "github.com/jpillora/go-tcp-proxy"
)

var (
	version = "0.0.0-src"
	connid  = uint64(0)

	localAddr       = flag.String("l", ":9999", "local address")
	remoteAddr      = flag.String("r", "localhost:80", "remote address")
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
			p = proxy.NewTLSUnwrapped(conn, laddr, raddr, *remoteAddr)
		} else {
			p = proxy.New(conn, laddr, raddr)
		}

		newOutboundPayload := createOutboundConnPayload(*outboundPayload)
		newIncomingPayload := createIncomingConnPayload(*incomingPayload)

		p.SetOutboundConnPayload(newOutboundPayload)
		p.SetIncomingConnPayload(newIncomingPayload)

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

func createOutboundConnPayload(outboundPayload string) string {
	return strings.Replace(outboundPayload, "[crlf]", "\r\n", -1)
}

func createIncomingConnPayload(incomingPayload string) string {
	return strings.Replace(incomingPayload, "[crlf]", "\r\n", -1)
}
