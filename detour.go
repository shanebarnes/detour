package main

import (
    "bufio"
    "bytes"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "io/ioutil"
    "log"
    "metrics"
    "net"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "strconv"
    "strings"
    "sync"
    "syscall"
    "time"
    "tokenbucket"
)

const _VERSION string = "0.3.0"
var _logger *log.Logger = log.New(os.Stdout, "", 0)

type Route struct {
    Bandwidth int64 `json:"bandwidth"`    // Bits per second
    Buffersize uint64 `json:"buffersize"` // Bytes
    Inspect bool `json:"inspect"`         // True = proxy, false = reverse proxy
    Guide string `json:"guide"`           // HTTP(S) probe to query a load balancer for backend addresses, response field name containing IP address, and static destination port
    Src string `json:"src"`
    Dst []string `json:"dst"`
}

type Itinerary struct {
    Map map[string]Route
}

func sigHandler(ch *chan os.Signal) {
    sig := <-*ch
    fmt.Println("Captured sig", sig)
    os.Exit(3)
}

func main() {
    sigs := make(chan os.Signal, 1)
    signal.Notify(sigs,
                  syscall.SIGHUP,
                  syscall.SIGINT,
                  syscall.SIGQUIT,
                  syscall.SIGABRT,
                  syscall.SIGKILL,
                  syscall.SIGSEGV,
                  syscall.SIGTERM)

    go sigHandler(&sigs)

    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

    itineraryFile := flag.String("itinerary", "itinerary.json", "file containing source and destination routes")
    flag.Usage = func() {
        fmt.Fprintf(os.Stderr, "version %s\n", _VERSION)
        fmt.Fprintln(os.Stderr, "usage:")
        flag.PrintDefaults()
    }
    flag.Parse()

    itinerary := loadItinerary(itineraryFile)

    var wg sync.WaitGroup
    wg.Add(len(itinerary.Map))

    for _, m := range itinerary.Map {
        go intercept(&wg, m)
    }

    wg.Wait()
}

func loadItinerary(fileName *string) Itinerary {

    file, _ := os.Open(*fileName)
    decoder := json.NewDecoder(file)
    itinerary := Itinerary{}
    err := decoder.Decode(&itinerary)
    if err != nil {
        log.Println(err.Error())
    }

    log.Println("Asking guides for directions")

    var wg sync.WaitGroup
    wg.Add(len(itinerary.Map))

    for i, m := range itinerary.Map {
        if len(m.Guide) > 0 {
            var uri, key string
            var port int
            args, _ := fmt.Sscanf(m.Guide, "%s %s %d", &uri, &key, &port)

            if args >= 2 {
                findDestinations(&wg, &m, uri, key, port)
                itinerary.Map[i] = m
            } else {
                wg.Done()
            }
        } else {
            wg.Done()
        }
    }

    wg.Wait()

    log.Printf("Finished asking guides for directions")

    return itinerary
}

func findDestinations(wg *sync.WaitGroup, route *Route, guide, key string, port int) {
    const TimeToLive = 10
    ask := TimeToLive
    count := 0

    for ask > 0 {
        if dst, err := askGuide(guide, key); err == nil {
            found := false
            for _, v := range route.Dst {
                if v == (dst + ":" + strconv.Itoa(port)) {
                    found = true
                    ask = ask - 1
                    break
                }
            }

            if !found {
                route.Dst = append(route.Dst, dst + ":" + strconv.Itoa(port))
                count = count + 1
                ask = TimeToLive
                log.Println(guide + ": found " + dst)
            } else {
                time.Sleep(250 * time.Millisecond)
            }
        } else {
            ask = ask - 1
        }
    }

    log.Printf("%s: found %d destinations\n", guide, count)
    wg.Done()
}

func askGuide(guide, key string) (string, error) {
    var dir string = ""
    tp := &http.Transport{ DisableKeepAlives: true, }
    client := &http.Client{ Transport: tp, Timeout: 4 * time.Second }
    r, err := client.Get(guide)

    if err == nil {
        if r.StatusCode == 200 {
            var dat map[string]interface{}
            body, _ := ioutil.ReadAll(r.Body)
            json.Unmarshal([]byte(string(body)), &dat)

            if val, ok := dat[key]; ok {
                if dst, ok := val.(string); ok {
                    dir = dst
                }
            }
        }

        r.Body.Close()
    }

    if len(dir) == 0 {
        err = errors.New("'" + key + "' key does not exist")
    }

    return dir, err
}

func intercept(wg *sync.WaitGroup, route Route) {
    listener, err := net.Listen("tcp", route.Src)
    if err == nil {
        log.Println("Listening on", route.Src)
        i := 0
        count := 0

        for {
            if con, err := listener.Accept(); err == nil {
                count = count + 1
                m := metrics.New(_logger, 1000000000, strconv.Itoa(count) + ",tx")
                if route.Inspect { // Proxy mode
                    go findHttpRoute(count, con, route.Bandwidth / 8, route.Buffersize, m)
                } else if len(route.Dst) > 0 { // Load balancer mode
                    go findRoute(count, con, route.Dst[i], route.Bandwidth / 8, route.Buffersize, m)
                    i = (i + 1) % len(route.Dst) // Round-robin for now
                }
            } else {
                log.Println(err.Error())
            }
        }
        listener.Close()
    } else {
        log.Println(err.Error())
    }

    wg.Done()
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
            log.Println(err.Error())
        }
    }

    return uri
}

func findHttpRoute(id int, src net.Conn, bandwidth int64, bufferSize uint64, m *metrics.Metrics) {
    buf := make([]byte, 65536)
    size, err := src.Read(buf) // Assume one read will include entire HTTP header

    if err == nil {
        reader := bufio.NewReader(strings.NewReader(string(buf[0:size])))
        header, _ := http.ReadRequest(reader)

        dstAddr := parseHttpRequestUri(header)
        dst, err := net.Dial("tcp", dstAddr)

        if err == nil {
            if header.Method == "CONNECT" {
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
            startDetour(id, src, dst, bandwidth, bufferSize, m)
        } else {
            src.Close()
            log.Println(err.Error())
        }
    } else {
        src.Close()
        log.Println(err.Error())
    }
}

func findRoute(id int, src net.Conn, dstAddr string, bandwidth int64, bufferSize uint64, m *metrics.Metrics) {
    dst, err := net.Dial("tcp", dstAddr)

    if err == nil {
        startDetour(id, src, dst, bandwidth, bufferSize, m)
    } else {
        src.Close()
        log.Println(err.Error())
    }
}

func startDetour(id int, src net.Conn, dst net.Conn, bandwidth int64, bufferSize uint64, m *metrics.Metrics) {
    log.Println("Opening route", id, ":", src.RemoteAddr().String(), "to", dst.RemoteAddr().String())

    var wg sync.WaitGroup
    wg.Add(1)
    go reroute(&wg, src, dst, bandwidth, bufferSize, m)
    reroute(nil, dst, src, bandwidth, bufferSize, metrics.New(_logger, 1000000000, strconv.Itoa(id) + ",rx"))
    wg.Wait()

    log.Println("Closing route", id, ":", src.RemoteAddr().String(), "to", dst.RemoteAddr().String())
}

func reroute(wg *sync.WaitGroup, src net.Conn, dst net.Conn, bandwidth int64, bufferSize uint64, m *metrics.Metrics) {
    tb := tokenbucket.New(uint64(bandwidth), 10 * uint64(bandwidth))
    buf := make([]byte, bufferSize)
    defer src.Close()
    defer dst.Close()

    for {
        bytes := tb.Remove(bufferSize)
        if bytes < bufferSize {
            tb.Return(bytes)
            time.Sleep(1 * time.Millisecond)
            continue
        }

        size, err := src.Read(buf)
        if err == nil {
            m.Add(int64(size))
            dst.Write(buf[0:size])
            if size < int(bufferSize) {
                tb.Return(bufferSize - uint64(size))
            }
        } else {
            break
        }
    }

    m.Dump()

    if wg != nil {
        wg.Done()
    }
}
