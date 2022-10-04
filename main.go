package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var cfg *Config
var ConnManager sync.Map
var SFUMap map[string]int

func run(ctx context.Context) error {
	var err error
	if cfg, err = parseArgs(ctx, os.Args); err != nil {
		log.Error("fails to parse args")
		os.Exit(-1)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg := sync.WaitGroup{}
	defer wg.Wait()

	// initialize sfu reference map
	SFUMap = make(map[string]int)
	for _, sfu := range cfg.SFUList {
		SFUMap[sfu] = 0
	}

	// signal handler
	go func(ctx context.Context) {
		ss := make(chan os.Signal, 0)
		signal.Notify(ss, syscall.SIGINT, syscall.SIGTERM)
		for s := range ss {
			log.Info(fmt.Sprintf("Quit for signal %v", s))
			cancel()
		}
	}(ctx)

	// for debugger
	go func() {
		if err := http.ListenAndServe(cfg.Debug, nil); err != nil {
			log.Error(fmt.Sprintf("debug server err %+v", err))
			return
		}
	}()

	wg.Add(1)
	go func(ctx context.Context, listen string) {
		defer wg.Done()

		conn, err := Listen(ctx, "udp", listen)
		if err != nil {
			log.Error(fmt.Sprintf("fail to listen port %v, err: %+v", listen, err))
			return
		}

		recvBytes := make([]byte, 2000)
		for ctx.Err() == nil {
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			count, addr, err := conn.ReadFrom(recvBytes)
			if err != nil {
				err, ok := err.(net.Error)
				if !ok {
					log.Error(fmt.Sprintf("fail to read, error: %+v", err))
					return
				}
				if !err.Timeout() && !err.Temporary() {
					log.Error(fmt.Sprintf("fail to read, error: %+v", err))
					return
				}
			}

			if count <= 0 {
				continue
			}

			key := addr.String()
			var slb *SLBConnection
			slbConn, ok := ConnManager.Load(key)
			if !ok {
				slb = newSLBConnection(ctx, conn, addr, key)
				if err = slb.initialize(); err != nil {
					log.Error("fail to initialize slb connection, error: %+v", err)
					return
				}
			} else {
				slb, ok = slbConn.(*SLBConnection)
				if !ok {
					log.Error(ctx, "fail to convert to slb conn")
					return
				}
			}

			if cfg.Mobility.Enable {
				diff := time.Since(slb.firstPkt)
				if diff > time.Duration(cfg.Mobility.Interval)*time.Second {
					// trigger mobility
					if cfg.Mobility.Mode == 1 {
						// use new port to connect original sfu
						slbMobility := newSLBConnection(ctx, conn, addr, key)
						if err = slbMobility.initializeWithSFU(slb.dstAddr.String()); err != nil {
							log.Error(fmt.Sprintf("fail to initialize slb connection, error:%+v", err))
							return
						}
						slb = slbMobility
					} else if cfg.Mobility.Mode == 2 {
						// use new port to connect new sfu
						slbMobility := newSLBConnection(ctx, conn, addr, key)
						originalIP := slb.dstAddr.String()
						var dstIP string
						for sfu, _ := range SFUMap {
							if sfu != originalIP {
								dstIP = sfu
							}
						}
						SFUMap[dstIP]++
						ConnManager.Delete(slb.key)
						if err = slbMobility.initializeWithSFU(dstIP); err != nil {
							log.Error(fmt.Sprintf("fail to initialize slb connection, error: %+v", err))
							return
						}
						slb = slbMobility
					}
				}
			}
			// send message to sfu
			recved := recvBytes[:count]
			if err = slb.sendToDst(recved); err != nil {
				log.Error(fmt.Sprintf("fail to send to dst, error: %+v", err))
				ConnManager.Delete(slb.key)
			}
		}
	}(ctx, cfg.ListenAddr)

	return err
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		os.Exit(-1)
	}
}
