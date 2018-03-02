package main

import (
	"net"
)

type MapTcp struct {
	Destinations []string
	Impl         MapImpl
}

func (m *MapTcp) FindRoute(guide GuideImpl, src net.Conn) (net.Conn, error) {
	i := m.Impl.RouteNumber % len(m.Destinations) // Round-robin for now

	dst, err := net.Dial("tcp", m.Destinations[i])
	m.Impl.Src = src
	m.Impl.Dst = dst

	return dst, err
}

func (m *MapTcp) Detour(role Role, buffer []byte) {
	m.Impl.Detour(role, buffer)
}

func (m *MapTcp) GetRouteNumber() int {
	return m.Impl.GetRouteNumber()
}

func (m *MapTcp) GetImpl() *MapImpl {
	return &m.Impl
}
