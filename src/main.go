package main

import (
	"encoding/json"
	"flag"
	"hash/fnv"
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

type UserConfig struct {
	Host           string   `json:"host"`
	Port           string   `json:"listen"`
	Backends       []string `json:"backends"`
	Algorithm      string   `json:"algorithm"`
	MaxConnections int      `json:"max_connections"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type Backend struct {
	Address           string
	IsHealthy         bool
	ActiveConnections int32
	mutex             sync.Mutex
}

type LoadBalancer struct {
	config          *UserConfig
	backends        []*Backend
	algo            Algorithm
	connectionCount int32
	index           int32
	backendCounts   []int32
}

type Algorithm int

const (
	RoundRobin Algorithm = iota
	LeastConnections
	IPHash
)

func parseAlgorithm(name string) Algorithm {
	switch strings.ToLower(name) {
	case "round_robin":
		return RoundRobin
	case "least_conn":
		return LeastConnections
	case "ip_hash":
		return IPHash
	default:
		log.Printf("Unknown algorithm %s, defaulting to round robin", name)
		return RoundRobin
	}
}

func (lb *LoadBalancer) getNextBackend(clientAddress string) (*Backend, func()) {
	var idx int
	var backend *Backend

	switch lb.algo {
	case RoundRobin:
		idx = int(atomic.AddInt32(&lb.index, 1)) % len(lb.backends)
		backend = lb.backends[idx]

	case LeastConnections:
		var minIdx int
		var minConn int32

		// check first backend
		lb.backends[0].mutex.Lock()
		minConn = lb.backends[0].ActiveConnections
		lb.backends[0].mutex.Unlock()

		for i := 1; i < len(lb.backends); i++ {
			lb.backends[i].mutex.Lock()
			currConn := lb.backends[i].ActiveConnections
			lb.backends[i].mutex.Unlock()

			if currConn < minConn {
				minConn = currConn
				minIdx = i
			}
		}

		backend = lb.backends[minIdx]

		// increment under lock
		backend.mutex.Lock()
		backend.ActiveConnections++
		backend.mutex.Unlock()

		idx = minIdx

	case IPHash:
		host, _, err := net.SplitHostPort(clientAddress)
		if err != nil {
			host = clientAddress
		}
		h := fnv.New32a()
		h.Write([]byte(host))
		hashVal := h.Sum32()

		idx = int(hashVal) % len(lb.backends)
		backend = lb.backends[idx]

	default:
		idx = int(atomic.AddInt32(&lb.index, 1)) % len(lb.backends)
		backend = lb.backends[idx]
	}

	// total LB connection count (atomic is fine here)
	atomic.AddInt32(&lb.connectionCount, 1)

	release := func() {
		backend.mutex.Lock()
		backend.ActiveConnections--
		backend.mutex.Unlock()

		atomic.AddInt32(&lb.connectionCount, -1)
	}

	return backend, release
}

func loadConfig(path string) (*UserConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config UserConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func main() {
	configPath := flag.String("config", "", "Path to config file (JSON)")
	flag.Parse()

	if strings.TrimSpace(*configPath) == "" {
		log.Fatal("Please provide a config file using -config flag")
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Backends) == 0 {
		log.Fatal("No backends provided in config")
	}

	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = ":1902"
	}

	var backendObjs []*Backend

	for _, address := range cfg.Backends {
		backendObjs = append(backendObjs,
			&Backend{
				Address:   address,
				IsHealthy: true,
			})
	}

	lb := &LoadBalancer{
		config:          cfg,
		algo:            parseAlgorithm(cfg.Algorithm),
		backends:        backendObjs,
		connectionCount: 0,
		index:           -1,
		backendCounts:   make([]int32, len(cfg.Backends)),
	}

	log.Printf("Akash started on %s with backends: %v", lb.config.Port, lb.backends)
	listenAddr := net.JoinHostPort(cfg.Host, cfg.Port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

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

			if atomic.LoadInt32(&lb.connectionCount) >= int32(lb.config.MaxConnections) {
				log.Printf("Max connections reached (%d), rejecting client %s", lb.config.MaxConnections, clientConn.RemoteAddr())
				clientConn.Close()
				continue
			}

			log.Printf("New client connected: %s", clientConn.RemoteAddr())
			backend, release := lb.getNextBackend(clientConn.RemoteAddr().String())

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

			timeout := time.Duration(lb.config.TimeoutSeconds) * time.Second
			clientConn.SetDeadline(time.Now().Add(timeout))
			backendConn.SetDeadline(time.Now().Add(timeout))

			atomic.AddInt32(&lb.connectionCount, 1)
			atomic.AddInt32(&backend.ActiveConnections, 1)
			log.Printf("Routing client [%s] -> Backend [%s]", clientConn.RemoteAddr(), backend.Address)

			go func() {
				defer clientConn.Close()
				defer backendConn.Close()
				release()
				io.Copy(backendConn, clientConn)

			}()

			go func() {
				defer clientConn.Close()
				defer backendConn.Close()
				release()
				io.Copy(clientConn, backendConn)

			}()

		}
	}()

	<-sigCh
	log.Println("🔻 Akash shutting down gracefully...")
}
