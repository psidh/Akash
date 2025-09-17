package config

import (
	core "Akash/core"
	"crypto/tls"
	"log"
	"sync/atomic"
)

var currentTLSConfig atomic.Value

func ReloadConfig(lb *core.LoadBalancer, configPath string) {
	log.Println("🔄 Reloading configuration...")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Printf("Failed to reload config: %v", err)
		return
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			log.Printf("Failed to reload TLS certs: %v", err)
		} else {
			currentTLSConfig.Store(&cert)
			log.Println("🔒 TLS certs reloaded")
		}
	}

	lb.Algo = core.ParseAlgorithm(cfg.Algorithm)
	lb.Config = cfg

	var newBackends []*core.Backend
	for _, backend := range cfg.Backends {

		found := false
		for _, oldB := range lb.Backends {
			if oldB.Address == backend.Address {
				newBackends = append(newBackends, oldB)
				found = true
				break
			}
		}
		if !found {
			newBackends = append(newBackends, &core.Backend{
				Address:   backend.Address,
				IsHealthy: true,
			})
		}
	}
	lb.Backends = newBackends

	lb.BackendCounts = make([]int32, len(lb.Backends))
	lb.BackendFails = make([]int32, len(lb.Backends))

	log.Printf("Configuration reloaded: %d backends, algorithm=%v", len(lb.Backends), lb.Algo)
}
