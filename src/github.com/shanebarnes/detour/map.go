package main

import (
    "net"
)

type MapImpl struct {
    Destination  net.Conn
    RouteCount   int
    Shortcut     Shortcut
}

type Map interface {
    FindRoute(src net.Conn) (net.Conn, error)
    GetRouteCount() int
    Detour(buffer []byte)
    GetImpl() *MapImpl
}

func (m *MapImpl) GetRouteCount() int {
    return m.RouteCount
}
