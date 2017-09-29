package main

import (
    "net"

    "github.com/twinj/uuid"
)

type ShortcutAzureBlob struct {
    block []byte
}

func (m *ShortcutAzureBlob) New() error {
    // This may need to be done in a singleton
    err := uuid.Init()
    return err
}

func (m *ShortcutAzureBlob) Take(dst net.Conn, buffer []byte) error {
    var err error = nil

    m.block = append(m.block, buffer...)

    _, err = dst.Write(buffer)

    _logger.Printf("Block size is now: %d\n", len(m.block))

    return err
}

func newRequestId() string {
    u1 := uuid.NewV1()
    return u1.String()
}
