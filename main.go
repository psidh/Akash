package main

import (
	config "Akash/config"
	core "Akash/core"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "", "Path to Config file (JSON)")
	flag.Parse()

	if strings.TrimSpace(*configPath) == "" {
		log.Fatal("Please provide a Config file using -Config flag")
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load Config: %v", err)
	}

	if len(cfg.Backends) == 0 {
		log.Fatal("No Backends provided in Config")
	}

	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = ":1902"
	}

	var backendObjs []*core.Backend
	for _, address := range cfg.Backends {
		backendObjs = append(backendObjs,
			&core.Backend{
				Address:   address,
				IsHealthy: true,
			})
	}

	lb := &core.LoadBalancer{
		Config:          cfg,
		Algo:            core.ParseAlgorithm(cfg.Algorithm),
		Backends:        backendObjs,
		ConnectionCount: 0,
		Index:           -1,
		BackendCounts:   make([]int32, len(cfg.Backends)),
	}
	backendAddrs := []string{}
	for _, b := range backendObjs {
		backendAddrs = append(backendAddrs, b.Address)
	}
	log.Printf("Akash started on %s with Backends: %v", lb.Config.Port, backendAddrs)
	core.StartHealthChecks(lb)

	listenAddr := net.JoinHostPort(cfg.Host, cfg.Port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	bufPool := sync.Pool{
		New: func() interface{} {
			return make([]byte, 32*1024)
		},
	}

	go func() {
		for {
			clientConn, err := listener.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
					break
				}
				log.Printf("Accept error: %v", err)
				continue
			}

			if atomic.LoadInt32(&lb.ConnectionCount) >= int32(lb.Config.MaxConnections) {
				log.Printf("Max connections reached (%d), rejecting client %s", lb.Config.MaxConnections, clientConn.RemoteAddr())
				clientConn.Close()
				continue
			}

			log.Printf("New client connected: %s", clientConn.RemoteAddr())
			backend, release := lb.GetNextBackend(clientConn.RemoteAddr().String())

			if backend == nil {
				log.Println("No backend available")
				clientConn.Close()
				continue
			}

			backendConn, err := net.Dial("tcp", backend.Address)
			if err != nil {
				log.Printf("Failed to connect to backend %s: %v", backend.Address, err)
				clientConn.Close()
				continue
			}

			timeout := time.Duration(lb.Config.TimeoutSeconds) * time.Second
			clientConn.SetDeadline(time.Now().Add(timeout))
			backendConn.SetDeadline(time.Now().Add(timeout))

			log.Printf("Routing client [%s] -> Backend [%v]", clientConn.RemoteAddr(), backend.Address)

			go proxy(clientConn, backendConn, &bufPool, release)
			go proxy(backendConn, clientConn, &bufPool, release)
		}
	}()

	<-sigCh
	log.Println("🔻 Akash shutting down gracefully...")
}

func proxy(src net.Conn, dst net.Conn, pool *sync.Pool, release func()) {
	defer release()

	tcpSrc, srcOK := src.(*net.TCPConn)
	tcpDst, dstOK := dst.(*net.TCPConn)

	buf := pool.Get().([]byte)
	defer pool.Put(buf)

	for {
		n, err := tcpSrc.Read(buf)
		if n > 0 {
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				break
			}
		}
		if err != nil {
			if err == io.EOF && srcOK && dstOK {
				tcpDst.CloseWrite()
			}
			break
		}
	}
}
