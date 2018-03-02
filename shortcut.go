package main

import (
    "net"
)

type ShortcutId int

const (
    Null      ShortcutId = iota
    AzureBlob
)

type ShortcutMeta struct {
    Id   ShortcutId
    Name string
}

var shortcutsSupported = [...]ShortcutMeta {
    { Id: Null,      Name: "shortcut_null" },
    { Id: AzureBlob, Name: "shortcut_azure_blob" },
}

type ShortcutImpl struct {
    // @todo Add map which contains connections, route number, etc.
    side [2]net.Conn // These should be wrapped so we can add atomic variable/condition variable that allows multi-thread access to a connection
// server side vs client side
    exits int64 // Partial or full dry run (profiling) of the shortcut
    block bool
}

type Shortcut interface {
    New(route int, client net.Conn, server net.Conn, exits int64, block bool) error
    Take(role Role, buffer []byte) (int, error)
}

func (s *ShortcutImpl) New(route int, client net.Conn, server net.Conn, exits int64, block bool) error {
    s.side[Client] = client
    s.side[Server] = server
    s.exits = exits
    s.block = block

    return nil
}

func (s *ShortcutImpl) Take(role Role, buffer []byte) (int, error) {
    var n int = -1
    var err error = nil

    // @todo Handle partial writes
    switch role {
    case Client:
        n, err = s.side[Server].Write(buffer)
    case Server:
        n, err = s.side[Client].Write(buffer)
    }

    return n, err
}
