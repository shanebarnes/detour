package main

import (
    "net"
)

type ShortcutNull struct {
    Impl ShortcutImpl
}

func (s *ShortcutNull) New(route int, client net.Conn, server net.Conn, exits int64, block bool) error {
    return s.Impl.New(route, client, server, exits, block)
}

func (s *ShortcutNull) Take(role Role, buffer []byte) (int, error) {
    return s.Impl.Take(role, buffer)
}
