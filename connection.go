package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type SLBConnection struct {
	ctx      context.Context
	cancel   context.CancelFunc
	key      string
	srcConn  net.PacketConn
	srcAddr  net.Addr
	dstConn  *net.UDPConn
	dstAddr  *net.UDPAddr
	lastPkt  time.Time
	firstPkt time.Time
	wg       sync.WaitGroup
}

func newSLBConnection(ctx context.Context, conn net.PacketConn, addr net.Addr, k string) *SLBConnection {
	v := &SLBConnection{
		srcConn:  conn,
		srcAddr:  addr,
		key:      k,
		lastPkt:  time.Now(),
		firstPkt: time.Now(),
	}
	v.ctx, v.cancel = context.WithCancel(ctx)
	return v
}

func (v *SLBConnection) initialize() error {
	// get least reference sfu
	var sfuAddr string
	leastRef := 100000000
	for sfu, ref := range SFUMap {
		if ref <= leastRef {
			sfuAddr = sfu
			leastRef = ref
		}
	}
	SFUMap[sfuAddr]++
	log.Info(fmt.Sprintf("transfer to %v", sfuAddr))
	var err error
	v.dstAddr, err = net.ResolveUDPAddr("udp", sfuAddr)
	if err != nil {
		return errors.New("fail to resolve sfu addr")
	}
	if v.dstConn, err = net.DialUDP("udp", nil, v.dstAddr); err != nil {
		return errors.New(fmt.Sprintf("dial udp dst. sfu:%v", sfuAddr))
	}

	ConnManager.Store(v.key, v)
	v.onTimer(v.ctx)
	v.recvFromDst(v.ctx)
	return nil
}

func (v *SLBConnection) initializeWithSFU(sfuAddr string) error {
	var err error
	v.dstAddr, err = net.ResolveUDPAddr("udp", sfuAddr)
	if err != nil {
		return errors.New("fail to resolve sfu addr")
	}
	if v.dstConn, err = net.DialUDP("udp", nil, v.dstAddr); err != nil {
		return errors.New(fmt.Sprintf("dial udp dst. sfu:%v", sfuAddr))
	}
	ConnManager.Store(v.key, v)
	v.onTimer(v.ctx)
	v.recvFromDst(v.ctx)
	return nil
}

func (v *SLBConnection) Close() {
	ConnManager.Delete(v.key)
	v.dstConn.Close()
	v.cancel()
	v.wg.Wait()
}

func (v *SLBConnection) sendToDst(b []byte) error {
	v.lastPkt = time.Now()
	count, err := v.dstConn.Write(b)
	if err != nil {
		return errors.New("fail to send to dst")
	}
	if count != len(b) {
		log.Error(fmt.Sprintf("send to dst, len = %v, buf len = %v", count, len(b)))
	}
	return nil
}

func (v *SLBConnection) onTimer(ctx context.Context) {
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		for {
			select {
			case <-ctx.Done():
				v.Close()
				return
			case <-time.After(5 * time.Second):
				diff := time.Now().Sub(v.lastPkt)
				if diff > 30*time.Second {
					log.Error("no packet for 30 seconds, close SLB connection")
					v.Close()
				}
			}
		}
	}()
}

func (v *SLBConnection) recvFromDst(ctx context.Context) {
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		defer v.Close()

		b := make([]byte, 2000)
		for ctx.Err() == nil {
			v.dstConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			count, err := v.dstConn.Read(b)
			if err != nil {
				err, ok := err.(net.Error)
				if !ok {
					log.Error(fmt.Sprintf("fails to read from dst, error:%+v", err))
					return
				}
				if !err.Timeout() && !err.Temporary() {
					log.Error(fmt.Sprintf("fail to read from dst, error:%+v", err))
				}
			}

			if 0 < count {
				v.lastPkt = time.Now()
				buf := b[:count]
				count, err = v.srcConn.WriteTo(buf, v.srcAddr)
				if err != nil {
					log.Error(fmt.Sprintf("fails to send src, error:%+v", err))
				}
				if count != len(buf) {
					log.Error(fmt.Sprintf("fails to send src msg, len=%v, buf len=%v", count, len(buf)))
				}
			}

			select {
			case <-v.ctx.Done():
				v.Close()
				return
			default:
			}
		}
	}()
}
