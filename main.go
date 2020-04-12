package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/shanebarnes/goto/logger"
	"github.com/shanebarnes/goto/tokenbucket"
	"github.com/twinj/uuid"
)

const _VERSION string = "0.5.0"

var _guide GuideImpl

type FastRoute struct {
	Exitno   int64  `json:"exitno"`   // The number of exits until the shortcut
	Shortcut string `json:"shortcut"` // Application protocol proxy
	Wormhole bool   `json:"wormhole"` // Application protocol emulation
}

type Route struct {
	Bandwidth  int64    `json:"bandwidth"`  // Bits per second (max travel speed)
	Buffersize uint64   `json:"buffersize"` // Bytes (max passengers)
	Delay      int64    `json:"delay"`      // Milliseconds (travel delay)
	Flow       int      `json:"flow"`       // Flow control one-way or two-way (one-way traffic will always flow from source to destination(s))
	Guide      string   `json:"guide"`      // HTTP(S) probe to query a load balancer for backend addresses, response field name containing IP address, and static destination port
	Inspect    bool     `json:"inspect"`    // True = proxy, false = reverse proxy
	SpeedLimit int64    `json:"speedLimit"` // Speed control in bits per second (maximum speed limit)
	Src        string   `json:"src"`        // Source/Point of Departure
	Dst        []string `json:"dst"`        // Destinations
}

type Itinerary struct {
	Map       map[string]Route `json:"map"`
	Shortcuts []FastRoute      `json:"shortcuts"`
}

func sigHandler(ch *chan os.Signal) {
	sig := <-*ch
	fmt.Println("Captured sig", sig)
	os.Exit(3)
}

func main() {
	uuid.RegisterGenerator(&uuid.GeneratorConfig{Resolution: 18465})

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

	logger.Init(log.Ldate|log.Ltime|log.Lmicroseconds, logger.Info, os.Stdout)

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
		logger.PrintlnError(err.Error())
	}

	_guide.LoadShortcuts(itinerary.Shortcuts)

	logger.PrintlnInfo("Asking guides for directions")

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

	logger.PrintlnInfo("Finished asking guides for directions")

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
				route.Dst = append(route.Dst, dst+":"+strconv.Itoa(port))
				count = count + 1
				ask = TimeToLive
				logger.PrintlnInfo(guide + ": found " + dst)
			} else {
				time.Sleep(250 * time.Millisecond)
			}
		} else {
			ask = 0
		}
	}

	logger.PrintlnInfo(guide, ": found", count, "destinations")
	wg.Done()
}

func askGuide(guide, key string) (string, error) {
	var dir string = ""
	tp := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tp, Timeout: 2 * time.Second}
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

	if err == nil && len(dir) == 0 {
		err = errors.New("'" + key + "' key does not exist")
	}

	return dir, err
}

func intercept(wg *sync.WaitGroup, route Route) {
	//ipAddr, err := net.ResolveIPAddr("ip4", route.Src)
	//if sock, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP); err == nil {
	//    syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4*1024*1024)
	//    syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 4*1024*1024)
	//    syscall.Bind(sock, ipAddr)
	//}

	listener, err := net.Listen("tcp", route.Src)

	// Add HTTP probe functions to a map class
	if err == nil {
		defer listener.Close()
		logger.PrintlnInfo("Listening on", route.Src)
		routeCount := 0
		for {
			if con, err := listener.Accept(); err == nil {
				go findRoute(con, &route, routeCount)
				routeCount++
			} else {
				logger.PrintlnError(err.Error())
			}
		}
	} else {
		logger.PrintlnError(err.Error())
	}

	wg.Done()
}

func findRoute(src net.Conn, route *Route, routeCount int) error {
	var res error = nil
	var mp Map = nil

	if route.Inspect { // Proxy mode (tunnel)
		mp = new(MapHttp)
	} else if len(route.Dst) > 0 { // Load balancer mode
		mpTcp := new(MapTcp)
		mpTcp.Destinations = route.Dst
		mp = mpTcp
	}

	if mp != nil {
		switch {
		case route.Flow == 1:
			mp.GetImpl().Flow = OneWay
		case route.Flow == 2:
			mp.GetImpl().Flow = TwoWay
		case route.Flow >= 3: // Temporary/random closure
			if (routeCount+1)%route.Flow == 0 {
				mp.GetImpl().Flow = Closed
			} else {
				mp.GetImpl().Flow = TwoWay
			}
		default:
			mp.GetImpl().Flow = Closed
		}

		mp.GetImpl().RouteNumber = routeCount
		if dst, err := mp.FindRoute(_guide, src); err == nil {
			startDetour(mp.GetRouteNumber(), src, dst, route, mp)
		} else {
			src.Close()
			logger.PrintlnError(err.Error())
		}
	} else {
		src.Close()
	}

	return res
}

func setTcpOptions(con net.Conn) {
	//listener.SetReadBuffer(4*1024*1024)
	//listener.SetWriteBuffer(4*1024*1024)
	if tcpCon, ok := con.(*net.TCPConn); ok == true {
		tcpCon.SetNoDelay(true)
	}
	//t, _ := con.(*net.TCPConn)
	//ptrVal := reflect.ValueOf(t)
	//val := reflect.Indirect(ptrVal)
	//val1conn := val.FieldByName("conn")
	//    val2 := reflect.Indirect(val1conn)

	//    // which is a net.conn from which we get the 'fd' field
	//    fdmember := val2.FieldByName("fd")
	//    val3 := reflect.Indirect(fdmember)

	//    // which is a netFD from which we get the 'sysfd' field
	//    netFdPtr := val3.FieldByName("sysfd")
	//    fmt.Printf("netFDPtr= %v\n", netFdPtr)
	//EnableTcpFastPath(t)
}

func startDetour(id int, src net.Conn, dst net.Conn, route *Route, mp Map) {
	logger.PrintlnInfo("Opening route", id, ":", src.RemoteAddr().String(), "to", dst.RemoteAddr().String(), "flow is", flowText[mp.GetFlow()])

	if mp.GetFlow() != Closed {
		setTcpOptions(src)
		setTcpOptions(dst)

		var wg sync.WaitGroup
		wg.Add(2)

		go reroute(&wg, src, dst, Client, route, mp)
		go reroute(&wg, dst, src, Server, route, mp)
		wg.Wait()
	} else {
		src.Close()
		dst.Close()
	}

	logger.PrintlnInfo("Closing route", id, ":", src.RemoteAddr().String(), "to", dst.RemoteAddr().String())
}

func reroute(wg *sync.WaitGroup, src net.Conn, dst net.Conn, role Role, route *Route, mp Map) {
	bandwidth := route.Bandwidth / 8
	bufferSize := route.Buffersize

	tag := ""
	switch role {
	case Client:
		tag = "CLIENT-" + strconv.Itoa(mp.GetRouteNumber())
	case Server:
		tag = "SERVER-" + strconv.Itoa(mp.GetRouteNumber())
	}

	metrics := MetricsNew(1000*1000*1000*1000, -1, tag)

	tbSize := uint64(bandwidth) * 10
	if bufferSize > uint64(bandwidth) {
		tbSize = bufferSize * 10
	}

	tb := tokenbucket.New(uint64(bandwidth), tbSize)
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
			if (role == Client && mp.GetFlow() == Closed) ||
				(role == Server && mp.GetFlow() != TwoWay) {
				logger.PrintlnDebug(tag, "flow is closed in this direction: blocking", size, "bytes")
			} else {
				logger.PrintlnDebug(tag, "flow is open in this direction: detouring", size, "bytes")
				mp.Detour(role, buf[:size])
			}
			metrics.Add(int64(size))

			if size < int(bufferSize) {
				tb.Return(bufferSize - uint64(size))
			}
		} else {
			logger.PrintlnInfo(tag, err.Error())
			break
		}
	}

	//metrics.Dump()

	if wg != nil {
		wg.Done()
	}
}
