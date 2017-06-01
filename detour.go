package main

import (
    "bufio"
    "bytes"
    "encoding/json"
    "flag"
    "fmt"
    "io/ioutil"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "strings"
    "sync"
    "syscall"
    "time"
    "tokenbucket"
)

const _VERSION string = "0.2.0"

type Route struct {
    Bandwidth int64 `json:"bandwidth"`    // Bits per second
    Buffersize uint64 `json:"buffersize"` // Bytes
    Inspect bool `json:"inspect"`         // True = proxy, false = reverse proxy
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

    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

    itineraryFile := flag.String("itinerary", "itinerary.json", "file containing source and destination routes")
    flag.Usage = func() {
        fmt.Fprintf(os.Stderr, "version %s\n", _VERSION)
        fmt.Fprintln(os.Stderr, "usage:")
        flag.PrintDefaults()
    }
    flag.Parse()

    itinerary := loadItinerary(itineraryFile)

    for _, m := range itinerary.Map {
        go intercept(m)
    }

    sigHandler(&sigs)
}

func loadItinerary(fileName *string) Itinerary {

    file, _ := os.Open(*fileName)
    decoder := json.NewDecoder(file)
    itinerary := Itinerary{}
    err := decoder.Decode(&itinerary)
    if err != nil {
        log.Println(err.Error())
    }

    return itinerary
}

func intercept(route Route) {
    listener, err := net.Listen("tcp", route.Src)
    if err == nil {
        log.Println("Listening on", route.Src)
        i := 0
        count := 0

        for {
            con, err := listener.Accept()
            if err == nil {
                count = count + 1

                if route.Inspect {
                    go findHttpRoute(count, con, route.Bandwidth / 8, route.Buffersize)
                } else {
                    // Round-robin for now
                    go findRoute(count, con, route.Dst[i], route.Bandwidth / 8, route.Buffersize)
                    i = (i + 1) % len(route.Dst)
                }
            } else {
                log.Println(err.Error())
            }
        }
        listener.Close()
    } else {
        log.Println(err.Error())
    }
}

func findHttpRoute(id int, src net.Conn, bandwidth int64, bufferSize uint64) {
    buf := make([]byte, 16384)
    size, err := src.Read(buf) // Assume one read will include entire HTTP header

    if err == nil {
        reader := bufio.NewReader(strings.NewReader(string(buf[0:size])))
        header, _ := http.ReadRequest(reader)
        dstAddr := header.RequestURI

        dst, err := net.Dial("tcp", dstAddr)

        if err == nil {
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
            buff := bytes.NewBuffer(nil)
            rsp.Write(buff)
            src.Write(buff.Bytes())
            startDetour(id, src, dst, bandwidth, bufferSize)
        } else {
            src.Close()
            log.Println(err.Error())
        }
    } else {
        src.Close()
        log.Println(err.Error())
    }
}

func findRoute(id int, src net.Conn, dstAddr string, bandwidth int64, bufferSize uint64) {
    dst, err := net.Dial("tcp", dstAddr)

    if err == nil {
        startDetour(id, src, dst, bandwidth, bufferSize)
    } else {
        src.Close()
        log.Println(err.Error())
    }
}

func startDetour(id int, src net.Conn, dst net.Conn, bandwidth int64, bufferSize uint64) {
    log.Println("Opening route", id, ":", src.RemoteAddr().String(), "to", dst.RemoteAddr().String())

    var wg sync.WaitGroup
    wg.Add(1)
    go reroute(&wg, src, dst, bandwidth, bufferSize)
    reroute(nil, dst, src, bandwidth, bufferSize)
    wg.Wait()

    log.Println("Closing route", id, ":", src.RemoteAddr().String(), "to", dst.RemoteAddr().String())
}

func reroute(wg *sync.WaitGroup, src net.Conn, dst net.Conn, bandwidth int64, bufferSize uint64) {
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
            dst.Write(buf[0:size])
            if size < int(bufferSize) {
                tb.Return(bufferSize - uint64(size))
            }
        } else {
            break
        }
    }

    if wg != nil {
        wg.Done()
    }
}
