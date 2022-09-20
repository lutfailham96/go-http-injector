package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
)

// Proxy - Manages a Proxy connection, piping data between local and remote.
type Proxy struct {
	sentBytes     uint64
	receivedBytes uint64
	laddr, raddr  *net.TCPAddr
	lconn, rconn  io.ReadWriteCloser
	saddr         string
	erred         bool
	errsig        chan bool
	tlsUnwrapp    bool
	tlsAddress    string

	Matcher  func([]byte)
	Replacer func([]byte) []byte

	// Settings
	Nagles    bool
	Log       Logger
	OutputHex bool

	maxFilterOutBuff uint64
	maxFilterInBuff  uint64

	payloadIncomingConn string
	payloadOutboundConn string

	reverseProxy bool
}

// New - Create a new Proxy instance. Takes over local connection passed in,
// and closes it when finished.
func New(lconn *net.TCPConn, laddr, raddr *net.TCPAddr, saddr string) *Proxy {
	return &Proxy{
		lconn:               lconn,
		laddr:               laddr,
		raddr:               raddr,
		saddr:               saddr,
		erred:               false,
		errsig:              make(chan bool),
		Log:                 NullLogger{},
		maxFilterOutBuff:    1024,
		maxFilterInBuff:     1024,
		payloadOutboundConn: "",
		payloadIncomingConn: "",
		reverseProxy:        false,
	}
}

// NewTLSUnwrapped - Create a new Proxy instance with a remote TLS server for
// which we want to unwrap the TLS to be able to connect without encryption
// locally
func NewTLSUnwrapped(lconn *net.TCPConn, laddr, raddr *net.TCPAddr, saddr, addr string) *Proxy {
	p := New(lconn, laddr, raddr, saddr)
	p.tlsUnwrapp = true
	p.tlsAddress = addr
	return p
}

type setNoDelayer interface {
	SetNoDelay(bool) error
}

// Start - open connection to remote and start proxying data.
func (p *Proxy) Start() {
	defer p.lconn.Close()

	var err error
	//connect to remote
	if p.tlsUnwrapp {
		p.rconn, err = tls.Dial("tcp", p.tlsAddress, nil)
	} else {
		p.rconn, err = net.DialTCP("tcp", nil, p.raddr)
	}
	if err != nil {
		p.Log.Warn("Remote connection failed: %s", err)
		return
	}
	defer p.rconn.Close()

	//nagles?
	if p.Nagles {
		if conn, ok := p.lconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
		if conn, ok := p.rconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
	}

	//display both ends
	p.Log.Info("Opened %s >>> %s", p.laddr.String(), p.raddr.String())

	//bidirectional copy
	go p.pipe(p.lconn, p.rconn)
	if !p.reverseProxy {
		go p.pipe(p.rconn, p.lconn)
	}

	//wait for close...
	<-p.errsig
	p.Log.Info("Closed (%d bytes sent, %d bytes recieved)", p.sentBytes, p.receivedBytes)
}

func (p *Proxy) err(s string, err error) {
	if p.erred {
		return
	}
	if err != io.EOF {
		p.Log.Warn(s, err)
	}
	p.errsig <- true
	p.erred = true
}

func (p *Proxy) SetOutboundConnPayload(payload string) {
	p.payloadOutboundConn = payload
}

func (p *Proxy) SetIncomingConnPayload(payload string) {
	p.payloadIncomingConn = payload
}

func (p *Proxy) SetReverseProxy(enabled bool) {
	p.reverseProxy = enabled
}

func (p *Proxy) SetMaxFilterInBuff(bufferSize uint64) {
	p.maxFilterInBuff = bufferSize
}

func (p *Proxy) SetMaxFilterOutBuff(bufferSize uint64) {
	p.maxFilterOutBuff = bufferSize
}

func (p *Proxy) createOutboundConnPayload() string {
	outPayload := strings.Replace(p.payloadOutboundConn, "[crlf]", "\r\n", -1)
	if p.saddr == "" {
		return outPayload
	}
	splitSaddr := strings.Split(p.saddr, ":")
	host := splitSaddr[0]
	port := splitSaddr[1]
	outPayload = strings.Replace(outPayload, "[host]", host, -1)
	outPayload = strings.Replace(outPayload, "[host_port]", fmt.Sprintf("%s:%s", host, port), -1)
	return outPayload
}

func (p *Proxy) createIncomingConnPayload() string {
	return strings.Replace(p.payloadIncomingConn, "[crlf]", "\r\n", -1)
}

func (p *Proxy) handleOutboundConn(src io.ReadWriter, buff []byte) ([]byte, bool) {
	clientWrite := false
	islocal := src == p.lconn
	if !islocal {
		return buff, false
	}

	if p.reverseProxy {
		if strings.Contains(strings.ToLower(string(buff)), "upgrade: websocket") {
			p.Log.Info("Upgrade connection to Websocket")
			buff = []byte("HTTP/1.1 101 Switching Protocols\r\n\r\n")
			clientWrite = true
			return buff, clientWrite
		}
	}

	if p.payloadOutboundConn == "" {
		return buff, clientWrite
	}

	if len(buff) > int(p.maxFilterOutBuff) {
		return buff, clientWrite
	}

	if bytes.Contains(buff, []byte("CONNECT ")) {
		outPayload := p.createOutboundConnPayload()
		buff = []byte(outPayload)
		p.Log.Info(string(buff))
	}

	return buff, clientWrite
}

func (p *Proxy) handleIncomingConn(src io.ReadWriter, buff []byte) []byte {
	isRemote := src != p.lconn

	if !isRemote {
		return buff
	}

	if p.payloadIncomingConn == "" {
		return buff
	}

	if len(buff) > int(p.maxFilterInBuff) {
		return buff
	}

	if bytes.Contains(buff, []byte("HTTP/1.")) {
		inPayload := p.createIncomingConnPayload()
		buff = []byte(inPayload)
		p.Log.Info(string(buff))
	}

	return buff
}

func (p *Proxy) pipe(src, dst io.ReadWriter) {
	islocal := src == p.lconn

	var dataDirection string
	if islocal {
		dataDirection = ">>> %d bytes sent%s"
	} else {
		dataDirection = "<<< %d bytes recieved%s"
	}

	var byteFormat string
	if p.OutputHex {
		byteFormat = "%x"
	} else {
		byteFormat = "%s"
	}

	//directional copy (64k buffer)
	buff := make([]byte, 0xffff)
	for {
		n, err := src.Read(buff)
		if err != nil {
			p.err("Read failed '%s'\n", err)
			return
		}
		b := buff[:n]

		var clientWrite bool
		b, clientWrite = p.handleOutboundConn(src, b)
		b = p.handleIncomingConn(src, b)

		//show output
		p.Log.Debug(dataDirection, n, "")
		p.Log.Trace(byteFormat, b)

		//write out result
		if clientWrite {
			if islocal {
				n, err = src.Write(b)
				go p.pipe(p.rconn, p.lconn)
			} else {
				n, err = dst.Write(b)
			}
		} else {
			n, err = dst.Write(b)
		}
		if err != nil {
			p.err("Write failed '%s'\n", err)
			return
		}
		if islocal {
			p.sentBytes += uint64(n)
		} else {
			p.receivedBytes += uint64(n)
		}
	}
}
