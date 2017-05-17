package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net"
    "os"
    "os/signal"
    "syscall"
)

const _VERSION string = "0.1.0"

type Itinerary struct {
    Src string `json:"src"`
    Dst []string `json:"dst"`
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
    intercept(itinerary)
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

func intercept(itinerary Itinerary) {
    listener, err := net.Listen("tcp", itinerary.Src)
    if err == nil {
        log.Println("Listening on", itinerary.Src)
        i := 0

        for {
            con, err := listener.Accept()
            if err == nil {
                go findRoute(con, itinerary.Dst[i])
                i = (i + 1) % len(itinerary.Dst)
            } else {
                log.Println(err.Error())
            }
        }
        listener.Close()
    } else {
        log.Println(err.Error())
    }
}

func findRoute(src net.Conn, dstAddr string) {
    dst, err := net.Dial("tcp", dstAddr)

    if err == nil {
        go reroute(src, dst)
        reroute(dst, src)
    } else {
        src.Close()
        log.Println(err.Error())
    }
}

func reroute(src net.Conn, dst net.Conn) {
    buf := make([]byte, 131072)
    defer src.Close()
    defer dst.Close()

    log.Println("Opening route:", src.RemoteAddr().String(), dst.RemoteAddr().String())

    for {
        size, err := src.Read(buf)
        if err == nil {
            dst.Write(buf[0:size])
        } else {
            break
        }
    }

    log.Println("Closing route:", src.RemoteAddr().String(), dst.RemoteAddr().String())
}
