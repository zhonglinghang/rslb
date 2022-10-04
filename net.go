package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
)

var reUsePort = 0xf

func control(network, address string, c syscall.RawConn) error {
	var err error
	c.Control(func(fd uintptr) {
		retErr := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		if retErr != nil {
			err = errors.New(fmt.Sprintf("set reuseaddr, fd: %v", fd))
			return
		}
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, reUsePort, 1)
	})
	return err
}

func Listen(ctx context.Context, network, laddr string) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: control}
	c, err := lc.ListenPacket(ctx, "udp", laddr)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("listen local addr: %v", laddr))
	}
	return c, nil
}

func Dial(network, laddr, raddr string) (net.Conn, error) {
	nla, err := net.ResolveUDPAddr(network, laddr)
	if err != nil {
		return nil, fmt.Errorf("resolving local addr")
	}
	d := net.Dialer{
		Control:   control,
		LocalAddr: nla,
	}
	return d.Dial(network, raddr)
}
