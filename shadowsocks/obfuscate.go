package shadowsocks

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/rand"
	"net"
	"time"
)

type Obfuscator interface {
	WrapConn(conn net.Conn) (net.Conn, error)
}

func NewObfuscator(obfs string) (Obfuscator, error) {
	switch obfs {
	case "":
		return nil, nil
	case "simple_http":
		return &simpleHttpObfuscator{}, nil
	default:
		return nil, errors.New("unsupported obfs: " + obfs)
	}
}

var (
	USER_AGENT = []string{
		"Mozilla/5.0 (Windows NT 6.3; WOW64; rv:40.0) Gecko/20100101 Firefox/40.0",
		"Mozilla/5.0 (Windows NT 6.3; WOW64; rv:40.0) Gecko/20100101 Firefox/44.0",
		"Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2228.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/535.11 (KHTML, like Gecko) Ubuntu/11.10 Chromium/27.0.1453.93 Chrome/27.0.1453.93 Safari/537.36",
		"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:35.0) Gecko/20100101 Firefox/35.0",
		"Mozilla/5.0 (compatible; WOW64; MSIE 10.0; Windows NT 6.2)",
		"Mozilla/5.0 (Windows; U; Windows NT 6.1; en-US) AppleWebKit/533.20.25 (KHTML, like Gecko) Version/5.0.4 Safari/533.20.27",
		"Mozilla/4.0 (compatible; MSIE 7.0; Windows NT 6.3; Trident/7.0; .NET4.0E; .NET4.0C)",
		"Mozilla/5.0 (Windows NT 6.3; Trident/7.0; rv:11.0) like Gecko",
		"Mozilla/5.0 (Linux; Android 4.4; Nexus 5 Build/BuildID) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/30.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (iPad; CPU OS 5_0 like Mac OS X) AppleWebKit/534.46 (KHTML, like Gecko) Version/5.1 Mobile/9A334 Safari/7534.48.3",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 5_0 like Mac OS X) AppleWebKit/534.46 (KHTML, like Gecko) Version/5.1 Mobile/9A334 Safari/7534.48.3",
	}
)

type simpleHttpObfuscator struct {
}

type simpleHttpObfsConn struct {
	net.Conn
	reqBuf        []byte
	resBuf        []byte
	reqHeaderSent bool
	resHeaderRecv bool
}

func (conn *simpleHttpObfsConn) Read(b []byte) (n int, err error) {
	if !conn.resHeaderRecv {
		conn.resBuf = make([]byte, 0)
		for len(conn.resBuf) < 8192 {
			buf := make([]byte, 8192)
			nn, err := conn.Conn.Read(buf)
			if err != nil {
				return 0, err
			}
			if nn == 0 {
				break
			}
			conn.resBuf = append(conn.resBuf, buf[:nn]...)
			index := bytes.Index(conn.resBuf, []byte("\r\n\r\n"))
			if index > -1 {
				conn.resHeaderRecv = true
				conn.resBuf = conn.resBuf[index+4:]
				break
			}
		}
	}
	if conn.resHeaderRecv {
		if conn.resBuf != nil {
			n = copy(b, conn.resBuf)
			if n >= len(conn.resBuf) {
				conn.resBuf = nil
			} else {
				conn.resBuf = conn.resBuf[n:]
			}
			return n, nil
		} else {
			return conn.Conn.Read(b)
		}
	} else {
		return 0, errors.New("can not find http_simple header")
	}
}

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func encodeHeader(b []byte) (s string) {
	hexstr := hex.EncodeToString(b)
	s = ""
	for i := 0; i < len(hexstr); i += 2 {
		s += "%" + hexstr[i:i+2]
	}
	return
}

func (conn *simpleHttpObfsConn) Write(b []byte) (n int, err error) {
	if conn.reqHeaderSent {
		return conn.Conn.Write(b)
	}

	headLen := len(b)
	if headLen > 16 {
		headLen = random(8, 16)
	}
	headData := b[:headLen]
	body := b[headLen:]
	host := "www.baidu.com:80"
	obfsHeaders := "GET /" + encodeHeader(headData) + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"User-Agent: " + USER_AGENT[random(0, len(USER_AGENT))] + "\r\n" +
		"Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8\r\nAccept-Language: en-US,en;q=0.8\r\nAccept-Encoding: gzip, deflate\r\nDNT: 1\r\nConnection: keep-alive\r\n\r\n"
	body = append([]byte(obfsHeaders), body...)
	for len(body) > 0 {
		nn, err := conn.Conn.Write(body)
		if err != nil {
			return 0, err
		}
		body = body[nn:]
	}
	conn.reqHeaderSent = true
	n = len(b)
	return
}

func (obfs *simpleHttpObfuscator) WrapConn(conn net.Conn) (net.Conn, error) {
	return &simpleHttpObfsConn{
		Conn:          conn,
		reqBuf:        nil,
		resBuf:        nil,
		reqHeaderSent: false,
		resHeaderRecv: false,
	}, nil
}
