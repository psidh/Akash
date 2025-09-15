package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
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
	connectionCount int32
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

	lb := &LoadBalancer{
		config:   cfg,
		backends: cfg.Backends,
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
			conn, err := listener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
					break
				}
				log.Printf("Accept error: %v", err)
				continue
			}
			log.Printf("New client connected: %s", conn.RemoteAddr())
			conn.Close()
		}
	}()

	<-sigCh
	log.Println("🔻 Akash shutting down gracefully...")
}
