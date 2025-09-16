package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
)

type UserConfig struct {
	ListenPort     string   `json:"listen"`
	Backends       []string `json:"backends"`
	Algorithm      string   `json:"algorithm"`
	MaxConnections int      `json:"max_connections"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type LoadBalancer struct {
	config          *UserConfig
	backends        []string
	algo            Algorithm
	connectionCount int32
	index           int32
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

func (lb *LoadBalancer) getNextBackend() string {
	switch lb.algo {
	case RoundRobin:
		idx := atomic.AddInt32(&lb.index, 1)
		return lb.backends[int(idx)%len(lb.backends)]

	case LeastConnections:
		return lb.backends[0]

	case IPHash:
		return lb.backends[0]

	default:
		idx := atomic.AddInt32(&lb.index, 1)
		return lb.backends[int(idx)%len(lb.backends)]
	}
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

	lb := &LoadBalancer{
		config:          cfg,
		algo:            parseAlgorithm(cfg.Algorithm),
		backends:        cfg.Backends,
		connectionCount: 0,
		index:           -1,
	}

	log.Printf("Akash started on %s with backends: %v", lb.config.ListenPort, lb.backends)

	listener, err := net.Listen("tcp", lb.config.ListenPort)
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

			log.Printf("New client connected: %s", clientConn.RemoteAddr())
			backendAddr := lb.getNextBackend()
			backendConn, err := net.Dial("tcp", backendAddr)
			if err != nil {
				log.Printf("Failed to connect to backend %s: %v", backendAddr, err)
				clientConn.Close()
				continue
			}
			log.Printf("Routing client [%s] -> Backend [%s]", clientConn.RemoteAddr(), backendAddr)

			go func() {
				defer clientConn.Close()
				defer backendConn.Close()
				io.Copy(backendConn, clientConn)

			}()

			go func() {
				defer clientConn.Close()
				defer backendConn.Close()
				io.Copy(clientConn, backendConn)

			}()

		}
	}()

	<-sigCh
	log.Println("🔻 Akash shutting down gracefully...")
}
