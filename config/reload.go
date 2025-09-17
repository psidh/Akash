package config

import (
	core "Akash/core"
	"log"
)

func ReloadConfig(lb *core.LoadBalancer, configPath string) {
	log.Println("🔄 Reloading configuration...")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Printf("Failed to reload config: %v", err)
		return
	}

	lb.Algo = core.ParseAlgorithm(cfg.Algorithm)
	lb.Config = cfg

	var newBackends []*core.Backend
	for _, address := range cfg.Backends {

		found := false
		for _, oldB := range lb.Backends {
			if oldB.Address == address {
				newBackends = append(newBackends, oldB)
				found = true
				break
			}
		}
		if !found {
			newBackends = append(newBackends, &core.Backend{
				Address:   address,
				IsHealthy: true,
			})
		}
	}
	lb.Backends = newBackends

	lb.BackendCounts = make([]int32, len(lb.Backends))
	lb.BackendFails = make([]int32, len(lb.Backends))

	log.Printf("Configuration reloaded: %d backends, algorithm=%v", len(lb.Backends), lb.Algo)
}
