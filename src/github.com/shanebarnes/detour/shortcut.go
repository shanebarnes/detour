package main

import (
    "net"
)

const (
    ShortcutNone   = 0
    ShortcutAzcopy = 1
)

type Shortcut interface {
    New() error
    Take(dst net.Conn, buffer []byte) error
}
