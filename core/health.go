package core

import (
	"log"
	"net"
	"time"
)

func StartHealthChecks(lb *LoadBalancer) {
	freq := time.Duration(lb.Config.HealthCheckFreq) * time.Second

	if freq == 0 {
		freq = 10 * time.Second
	}

	go func() {
		for {
			for _, backend := range lb.Backends {
				go checkBackend(backend, lb.Config)

			}
			time.Sleep(freq)
		}
	}()
}

func checkBackend(backend *Backend, userConfig *UserConfig) {
	conn, err := net.DialTimeout("tcp", backend.Address, 2*time.Second)

	if err != nil {
		setBackendHealth(backend, false)
		return
	}

	conn.Close()
	setBackendHealth(backend, true)
}

func setBackendHealth(backend *Backend, healthy bool) {

	backend.mutex.Lock()

	defer backend.mutex.Unlock()

	if backend.IsHealthy != healthy {
		log.Printf("Backend %s health changed → %v", backend.Address, healthy)
	}
	backend.IsHealthy = healthy
}
