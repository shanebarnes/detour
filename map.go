package main

import (
	"net"
)

type Role int

const (
	Client Role = iota
	Server
)

type Flow int

const (
	Closed Flow = iota
	OneWay
	TwoWay
)

var flowText = map[Flow]string{
	Closed: "Closed",
	OneWay: "One-Way",
	TwoWay: "Two-Way",
}

type MapImpl struct {
	Flow        Flow
	Src         net.Conn // Arrival connection
	Dst         net.Conn // Departure connection
	RouteNumber int
	Shortcut    Shortcut
}

type Map interface {
	Detour(role Role, buffer []byte)
	GetImpl() *MapImpl
	FindRoute(guide GuideImpl, src net.Conn) (net.Conn, error)
	GetFlow() Flow
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

func (m *MapImpl) GetFlow() Flow {
	return m.Flow
}

func (m *MapImpl) GetRouteNumber() int {
	return m.RouteNumber
}
