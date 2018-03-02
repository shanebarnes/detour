package main

import (
    "net"
)

type Role int

const (
    Client Role = iota
    Server
)

type MapImpl struct {
    Src         net.Conn // Arrival connection
    Dst         net.Conn // Departure connection
    RouteNumber int
    Shortcut    Shortcut
}

type Map interface {
    Detour(role Role, buffer []byte)
    GetImpl() *MapImpl
    FindRoute(guide GuideImpl, src net.Conn) (net.Conn, error)
    GetRouteNumber() int
}

func (m *MapImpl) Detour(role Role, buffer []byte) {
    switch role {
    case Client:
        m.Dst.Write(buffer)
    case Server:
        m.Src.Write(buffer)
    }
}

func (m *MapImpl) GetRouteNumber() int {
    return m.RouteNumber
}
