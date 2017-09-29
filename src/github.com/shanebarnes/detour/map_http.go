package main

import (
    "bufio"
    "bytes"
    "io/ioutil"
    "net"
    "net/http"
    "net/url"
    "strings"
)

const roleClient = "[CLIENT REQ]"
const roleServer = "[CLIENT RSP]"

type MapHttp struct {
    Role          string
    ContentLength int64
    Impl          MapImpl
}

func (m *MapHttp) FindRoute(src net.Conn) (net.Conn, error) {
    var res net.Conn = nil

    buf := make([]byte, 65536)
    size, err := src.Read(buf) // Assume one read will include entire HTTP header

    if err == nil {
        reader := bufio.NewReader(strings.NewReader(string(buf[0:size])))
        request, _ := http.ReadRequest(reader)

        dstAddr := parseHttpRequestUri(request)
        dst, err := net.Dial("tcp", dstAddr)

        m.Role = roleClient

        if strings.HasPrefix(request.UserAgent(), "AzCopy") {
            m.Impl.Shortcut = new(ShortcutAzureBlob)
            _logger.Println("Detected AzCopy client")
        }

        _logger.Println(request)
        _logger.Printf("%s Content Length: %d\n", m.Role, request.ContentLength)

        if err == nil {
            if request.Method == "CONNECT" {
                body := ""
                rsp := &http.Response {
                    Status:        "200 OK",
                    StatusCode:    200,
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
                dst.Write(buf[0:size])
            }

            m.Impl.Destination = dst
            res = dst
        }
    }

    return res, err
}

func (m *MapHttp) Detour(buffer []byte) {
    if m.Impl.Shortcut != nil {
        m.Impl.Shortcut.Take(m.Impl.Destination, buffer)
    }

    if m.ContentLength == 0 {
        payload := string(buffer)
        if r := getHttpRequest(&payload); r != nil {
            m.ContentLength = r.ContentLength
            m.Role = roleClient
            _logger.Println(r)
            _logger.Printf("%s Content Length: %d\n", m.Role, m.ContentLength)
        } else if r := getHttpResponse(&payload); r != nil {
            m.ContentLength = r.ContentLength
            m.Role = roleServer
            _logger.Println(r)
            _logger.Printf("%s Content Length: %d\n", m.Role, m.ContentLength)
        }
    } else { // HTTP continuation
        m.ContentLength = m.ContentLength - int64(len(buffer))
        //_logger.Println("%s Content Length: %d (bytes read %d)\n", direction, contentLength, size)
    }

    m.Impl.Destination.Write(buffer)
}

func (m *MapHttp) GetImpl() *MapImpl {
    return &m.Impl
}

func (m *MapHttp) GetRouteCount() int {
    return m.Impl.GetRouteCount()
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
            _logger.Println(err.Error())
        }
    }

    return uri
}

func getHttpRequest(request *string) *http.Request {
    var req *http.Request = nil

    reader := bufio.NewReader(strings.NewReader(*request))

    if r, err := http.ReadRequest(reader); err == nil {
        req = r
    }

    return req
}

func getHttpResponse(response *string) *http.Response {
    var rsp *http.Response = nil

    reader := bufio.NewReader(strings.NewReader(*response))

    if r, err := http.ReadResponse(reader, nil); err == nil {
        rsp = r
    }

    return rsp
}
