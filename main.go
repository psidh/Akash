package main

import (
	"Akash/config"
	"Akash/core"
	"Akash/metrics"
	"crypto/tls"
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
)

var currentTLSConfig atomic.Value

func main() {
	// -------------------- config --------------------
	configPath := flag.String("config", "", "Path to Config file (JSON)")
	flag.Parse()

	if strings.TrimSpace(*configPath) == "" {
		log.Fatal("Please provide a Config file using -config flag")
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load Config: %v", err)
	}

	if len(cfg.Backends) == 0 {
		log.Fatal("No backends provided in config")
	}

	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = "1902"
	}

	// -------------------- init loadbalancer --------------------
	var backendObjs []*core.Backend
	for _, backend := range cfg.Backends {
		backendObjs = append(backendObjs,
			&core.Backend{
				Address:   backend.Address,
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
		BackendFails:    make([]int32, len(cfg.Backends)),
		PathRoutes:      make(map[string]*core.Backend),
	}

	// -------------------- start listener --------------------
	listenAddr := net.JoinHostPort(cfg.Host, cfg.Port)
	var listener net.Listener

	// -------------------- security jargon --------------------
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS cert/key: %v", err)
		}
		tlsConfig := &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				c := currentTLSConfig.Load().(*tls.Certificate)
				return c, nil
			},
		}
		currentTLSConfig.Store(&cert)
		listener, err = tls.Listen("tcp", listenAddr, tlsConfig)
		log.Printf("[INFO] TLS listener started on %s", listenAddr)
	} else {
		listener, err = net.Listen("tcp", listenAddr)
		log.Printf("[INFO] TCP listener started on %s", listenAddr)
	}

	if err != nil {
		log.Fatalf("[ERROR] Failed to listen: %v", err)
	}

	// -------------------- signal handling --------------------
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	var shuttingDown atomic.Bool
	var wg sync.WaitGroup
	activeConns := sync.Map{}
	bufPool := sync.Pool{New: func() interface{} { return make([]byte, 32*1024) }}

	metrics.StartMetricsServer(":9100")
	log.Println("[INFO] Metrics server started on :9100")

	// -------------------- accept loop --------------------
	go func() {
		for {
			clientConn, err := listener.Accept()
			if err != nil {
				if shuttingDown.Load() {
					log.Println("[INFO] Listener closed, stopping accept loop")
					return
				}
				if strings.Contains(err.Error(), "use of closed network connection") {
					log.Println("[INFO] Listener closed, stopping accept loop")
					return
				}
				log.Printf("[WARN] Accept error: %v", err)
				continue
			}

			if shuttingDown.Load() {
				log.Printf("[INFO] Shutting down, closing new connection: %s", clientConn.RemoteAddr())
				clientConn.Close()
				continue
			}

			wg.Add(1)
			metrics.ActiveConns.Inc()
			activeConns.Store(clientConn, struct{}{})
			log.Printf("[INFO] New client connected: %s", clientConn.RemoteAddr())

			// -------------------- get backend --------------------
			backend, _, release := lb.GetNextBackend(clientConn.RemoteAddr().String(), "/")
			if backend == nil {
				log.Printf("[WARN] No backend available, closing connection %s", clientConn.RemoteAddr())
				clientConn.Close()
				wg.Done()
				activeConns.Delete(clientConn)
				continue
			}
			backendAddr := backend.Address

			backendConn, err := net.Dial("tcp", backendAddr)
			if err != nil {
				log.Printf("[ERROR] Failed to connect backend %s: %v", backendAddr, err)
				clientConn.Close()
				metrics.PerBackendFails.WithLabelValues(backendAddr).Inc()
				wg.Done()
				activeConns.Delete(clientConn)
				release()
				continue
			}

			activeConns.Store(backendConn, struct{}{})
			metrics.PerBackendServed.WithLabelValues(backendAddr).Inc()
			log.Printf("[INFO] Connected client %s -> backend %s", clientConn.RemoteAddr(), backendAddr)
			log.Printf("event=route client=%s backend=%s active_conns=%d", clientConn.RemoteAddr(), backend.Address, atomic.LoadInt32(&lb.ConnectionCount))

			// -------------------- proxy goroutine --------------------
			go func(c, b net.Conn, releaseFunc func()) {
				defer wg.Done()
				defer c.Close()
				defer b.Close()
				defer metrics.ActiveConns.Dec()
				defer activeConns.Delete(c)
				defer activeConns.Delete(b)
				defer releaseFunc()

				log.Printf("[INFO] Starting proxy: client=%s backend=%s", c.RemoteAddr(), b.RemoteAddr())

				var proxyWg sync.WaitGroup
				proxyWg.Add(2)

				copyFunc := func(dst, src net.Conn) {
					defer proxyWg.Done()
					buf := bufPool.Get().([]byte)
					defer bufPool.Put(buf)
					n, err := io.CopyBuffer(dst, src, buf)
					log.Printf("[INFO] %s -> %s copy finished: %d bytes, err=%v", src.RemoteAddr(), dst.RemoteAddr(), n, err)
					if tcp, ok := dst.(*net.TCPConn); ok {
						tcp.CloseWrite()
					}
					if tcp, ok := src.(*net.TCPConn); ok {
						tcp.CloseRead()
					}
				}

				go copyFunc(b, c)
				go copyFunc(c, b)

				proxyWg.Wait()
				log.Printf("[INFO] Proxy finished: client=%s backend=%s", c.RemoteAddr(), b.RemoteAddr())
			}(clientConn, backendConn, release)
		}
	}()

	sig := <-sigCh
	log.Printf("[INFO] Signal received: %v. Shutting down...", sig)
	shuttingDown.Store(true)
	listener.Close()

	activeConns.Range(func(key, _ interface{}) bool {
		log.Printf("[INFO] Closing active connection: %v", key.(net.Conn).RemoteAddr())
		key.(net.Conn).Close()
		return true
	})

	wg.Wait()
	log.Println("[INFO] All connections closed. Akash shutdown complete.")
}
