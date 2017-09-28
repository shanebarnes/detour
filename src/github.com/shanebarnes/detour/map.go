package main

import (
    "net"
)

type MapImpl struct {

}

type Map interface {
    FindRoute(src net.Conn) (net.Conn, error)
    Inspect(buffer []byte)
    GetImpl() *MapImpl
}
