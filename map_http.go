package main

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"

	"github.com/shanebarnes/goto/logger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)


const (
	eom string = "\r\n\r\n"
	indent string = "    "

	methodConnect = "CONNECT"

	pseudoHdrAuth = ":authority"
	pseudoHdrMethod = ":method"
	pseudoHdrPath = ":path"
	pseudoHdrScheme = ":scheme"
)

type MapHttp struct {
	Impl MapImpl
}

func (m *MapHttp) createDstConn(guide GuideImpl, src net.Conn, hostPort, userAgent string) (net.Conn, error) {
	dst, err := net.Dial("tcp", hostPort)

	if err == nil {
		m.Impl.Shortcut = guide.FindShortcut(m.GetRouteNumber(), Client, userAgent, src, dst)
		m.Impl.Src = src
		m.Impl.Dst = dst
	}

	return dst, err
}

func (m *MapHttp) findHttp1Route(guide GuideImpl, src net.Conn, buf *bytes.Buffer) (string, net.Conn, error) {
	var dst net.Conn
	var addr string
	var err error
	var request *http.Request

	reader := bufio.NewReader(strings.NewReader(string(buf.Bytes())))
	if request, err = http.ReadRequest(reader); err == nil {
		logger.PrintlnInfo("Found a", request.Proto, "route for a", request.Method, "request to", request.RequestURI)

		if request.Method == methodConnect {
			addr = request.RequestURI

			body := ""
			hdr := make(http.Header, 0)
			hdr.Add("X-Detour-Version", _VERSION)

			rsp := &http.Response{
				Status:        "200 OK",
				StatusCode:    http.StatusOK,
				Proto:         "HTTP/1.1",
				ProtoMajor:    1,
				ProtoMinor:    1,
				Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
				ContentLength: int64(len(body)),
				Request:       nil,
				Header:        hdr,
			}
			rspBuf := bytes.NewBuffer(nil)
			rsp.Write(rspBuf)
			src.Write(rspBuf.Bytes())
		} else {
			url, err := url.Parse(request.RequestURI)

			if err == nil {
				//if request.ProtoAtLeast(2, 0) {
				//}

				if strings.ContainsAny(url.Host, ":") {
					addr = url.Host
				} else {
					switch url.Scheme {
					case "http":
						addr = url.Host + ":80"
					case "https":
						addr = url.Host + ":443"
					default:
						addr = url.Host
					}
				}
			} else {
				logger.PrintlnError(err.Error())
			}
		}

		dst, err = m.createDstConn(guide, src, addr, request.UserAgent())
	}

	return request.Method, dst, err
}

// Test: export http_proxy=<host>:<port>; curl -I --http2 --http2-prior-knowledge http://www.google.com/
func (m *MapHttp) findHttp2Route(guide GuideImpl, src net.Conn, buf *bytes.Buffer) (string, net.Conn, error) {
	var method string
	var dst net.Conn
	var err error

	prefaceLen := len(http2.ClientPreface)

	if buf.Len() < prefaceLen {
		err = syscall.EPROTONOSUPPORT // HTTP/2.0 protocol not supported since client preface requires 24 bytes
	} else if strings.HasPrefix(string(buf.Bytes()[:prefaceLen]), http2.ClientPreface) {
		buf.Next(prefaceLen)
		frameReader := http2.NewFramer(ioutil.Discard, buf)

		var frame http2.Frame
		var addr, authority, path, scheme string
		decoder := hpack.NewDecoder(2048, nil)
		for err == nil {
			frame, err = frameReader.ReadFrame()

			if err == nil {
				switch f := frame.(type) {
				case *http2.SettingsFrame:
					logger.PrintlnDebug("settings:")
					f.ForeachSetting(func(s http2.Setting) error {
						logger.PrintlnDebug(indent, s.ID, "=", s.Val)
						return nil
					})
				case *http2.WindowUpdateFrame:
					logger.PrintlnDebug("window update:",)
					logger.PrintlnDebug(indent, "increment", "=", f.Increment)
				case *http2.HeadersFrame:
					logger.PrintlnDebug("headers:")
					hf, _ := decoder.DecodeFull(f.HeaderBlockFragment())
					for _, h := range hf {
						switch h.Name {
						case pseudoHdrAuth:
							authority = h.Value
						case pseudoHdrMethod:
							method = h.Value
						case pseudoHdrPath:
							path = h.Value
						case pseudoHdrScheme:
							scheme = h.Value
						}
						logger.PrintlnDebug(indent, h.Name, "=", h.Value)
					}
				default:
					logger.PrintlnDebug("frame:", f)
				}
			} else if err != io.EOF {
				logger.PrintlnError("Failed to read HTTP/2.0 frame:", err)
			}
		}

		if err == nil || err == io.EOF {
			logger.PrintlnInfo("Found a HTTP/2.0 route for a", method, "request to", path)

			if _, _, err = net.SplitHostPort(authority); err == nil {
				addr = authority
			} else if scheme == "http" {
				addr = authority + ":80"
			} else if scheme == "https" {
				addr = authority + ":443"
			}

			dst, err = m.createDstConn(guide, src, addr, "user-agent")
		}
	} else {
		err = syscall.ENOPROTOOPT // HTTP/2.0 protocol not available
	}

	return method, dst, err
}

func (m *MapHttp) FindRoute(guide GuideImpl, src net.Conn) (net.Conn, error) {
	var err error
	var dst net.Conn
	var method string

	b := make([]byte, 64*1024*1024)
	buf := bytes.NewBuffer(make([]byte, 0))
	size := 0

	// Assume one read will include entire HTTP header
	if size, err = src.Read(b); err == nil {
		buf.Write(b[:size])
		logger.PrintlnDebug("Read", buf.Len(), "bytes")

		if method, dst, err = m.findHttp2Route(guide, src, buf); err != nil {
			method, dst, err = m.findHttp1Route(guide, src, buf)
		}

		// Only forward original request if not a CONNECT request
		if err == nil && method != methodConnect {
			_, err = m.Impl.Shortcut.Take(Client, b[:size])
		}
	}

	return dst, err
}

func (m *MapHttp) Detour(role Role, buffer []byte) {
	m.Impl.Shortcut.Take(role, buffer)
}

func (m *MapHttp) GetImpl() *MapImpl {
	return &m.Impl
}

func (m *MapHttp) GetRouteNumber() int {
	return m.Impl.GetRouteNumber()
}

func GetHttpRequest(request *string) (*http.Request, int) {
	var req *http.Request = nil
	var n int = 0

	reader := bufio.NewReader(strings.NewReader(*request))

	if r, err := http.ReadRequest(reader); err == nil {
		req = r
		n = strings.Index(*request, eom) + len(eom)
	}

	return req, n
}

func GetHttpResponse(response *string) (*http.Response, int) {
	var rsp *http.Response = nil
	var n int = 0

	reader := bufio.NewReader(strings.NewReader(*response))

	if r, err := http.ReadResponse(reader, nil); err == nil {
		rsp = r
		n = strings.Index(*response, eom) + len(eom)
	}

	return rsp, n
}
