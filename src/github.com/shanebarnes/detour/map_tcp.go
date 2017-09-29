package main

import (
    "net"
)

type MapTcp struct {
    Destinations []string
    Impl           MapImpl
}

func (m *MapTcp) FindRoute(src net.Conn) (net.Conn, error) {
    i := m.Impl.RouteCount % len(m.Destinations) // Round-robin for now

    dst, err := net.Dial("tcp", m.Destinations[i])
    m.Impl.Destination = dst

    return dst, err
}

func (m *MapTcp) Detour(buffer []byte) {
    m.Impl.Destination.Write(buffer)
}

func (m *MapTcp) GetRouteCount() int {
    return m.Impl.GetRouteCount()
}

func (m *MapTcp) GetImpl() *MapImpl {
    return &m.Impl
}
