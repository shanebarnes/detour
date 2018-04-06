package main

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/shanebarnes/goto/logger"
)

const eom string = "\r\n\r\n"

type MapHttp struct {
	Impl MapImpl
}

func (m *MapHttp) FindRoute(guide GuideImpl, src net.Conn) (net.Conn, error) {
	var err error = nil
	var res net.Conn = nil

	buf := make([]byte, 65536)
	size := 0

	// Assume one read will include entire HTTP header
	if size, err = src.Read(buf); err == nil {
		reader := bufio.NewReader(strings.NewReader(string(buf[0:size])))

		var request *http.Request = nil
		if request, err = http.ReadRequest(reader); err == nil {

			dstAddr := parseHttpRequestUri(request)
			var dst net.Conn = nil
			dst, err = net.Dial("tcp", dstAddr)

			m.Impl.Shortcut = guide.FindShortcut(m.GetRouteNumber(), Client, request.UserAgent(), src, dst)

			if err == nil {
				if request.Method == "CONNECT" {
					body := ""
					rsp := &http.Response{
						Status:        "200 OK",
						StatusCode:    http.StatusOK,
						Proto:         "HTTP/1.1",
						ProtoMajor:    1,
						ProtoMinor:    1,
						Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
						ContentLength: int64(len(body)),
						Request:       nil,
						Header:        make(http.Header, 0),
					}
					rspBuf := bytes.NewBuffer(nil)
					rsp.Write(rspBuf)
					src.Write(rspBuf.Bytes())
				} else {
					m.Impl.Shortcut.Take(Client, buf[:size])
				}

				m.Impl.Src = src
				m.Impl.Dst = dst
				res = dst
			}
		}
	}

	return res, err
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

func parseHttpRequestUri(header *http.Request) string {
	var uri string
	if header.Method == "CONNECT" {
		uri = header.RequestURI
	} else {
		url, err := url.Parse(header.RequestURI)

		if err == nil {
			if strings.ContainsAny(url.Host, ":") {
				uri = url.Host
			} else {
				switch url.Scheme {
				case "http":
					uri = url.Host + ":80"
				case "https":
					uri = url.Host + ":443"
				default:
					uri = url.Host
				}
			}
		} else {
			logger.PrintlnError(err.Error())
		}
	}

	return uri
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
