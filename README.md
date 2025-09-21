# Akash

Akash is a Layer 4 load balancer written in Go.
It supports multiple routing algorithms, active health checks, Prometheus metrics, TLS termination, and path-based routing.

> ⚠️ Warning: Akash is not production-ready. It was built purely for fun and learning purposes. It lacks stability, security, and robustness of a production-grade load balancer.

Website: [akash.psidharth.dev](https://akash.psidharth.dev/)

![alt text](./Akash.jpeg)

---

#### Tech Stack

[![My Skills](https://skillicons.dev/icons?i=golang,prometheus,grafana,docker)](https://skillicons.dev)

---

## Performance vs HAProxy (L4)

We benchmarked **Akash** against **HAProxy**, the industry standard L4 load balancer.
The results show Akash performs on par with HAProxy in throughput and latency.

| Metric           | Akash        | HAProxy (L4) |
| ---------------- | ------------ | ------------ |
| Total Requests   | 1,000,000    | 1,000,000    |
| Throughput       | 71,290 req/s | 71,585 req/s |
| Median Latency   | 2.1 ms       | 1.7 ms       |
| 95th Percentile  | 6.3 ms       | 7.2 ms       |
| 99th Percentile  | 12.1 ms      | 14.7 ms      |
| Max Latency      | 129 ms       | 95.9 ms      |
| Data Transferred | 40 MB        | 40 MB        |
| Success Rate     | 99.995%      | 99.995%      |

---

### 📊 Benchmark Insights

- Akash matches HAProxy in throughput (within \~0.5%).
- Latency distribution is nearly identical, with Akash slightly better at higher percentiles.
- Max latency spikes are different but within acceptable range.

For a Go-based project vs HAProxy’s C engine, Akash proves competitive.

---

## Features

- **Routing Algorithms**

  - Round Robin
  - Least Connections
  - IP Hash
  - Weighted Round Robin

- **Health Checks**

  - Regular TCP-based health checks
  - Automatic marking of backends as healthy/unhealthy

- **Metrics**

  - Exposes Prometheus metrics on `:9100/metrics`
  - Tracks active connections, per-backend requests, and per-backend failures

- **TLS Support**

  - Optional TLS termination with provided certificates

- **Path-Based Routing**

  - Route specific URL paths to specific backends

- **Graceful Shutdown**

  - Handles termination signals
  - Closes active connections before exit

---

## Getting Started

### Prerequisites

- Go 1.21 or higher
- A valid `config.json` file (see example below)

### Build

```bash
git clone https://github.com/your-username/akash.git
cd akash
go build -o akash
```

### Run

```bash
./akash -config config.json
```

---

## Configuration

Akash is configured using a JSON file. Below is an example:

```json
{
  "host": "0.0.0.0",
  "listen": "1902",
  "algorithm": "round_robin",
  "max_connections": 1000,
  "timeout_seconds": 5,
  "health_check_path": "/health",
  "health_check_port": "80",
  "health_check_freq": 10,
  "tls_cert_file": "server.crt",
  "tls_key_file": "server.key",
  "Backends": [
    {
      "address": "127.0.0.1:8081",
      "weight": 1,
      "paths": ["/api"]
    },
    {
      "address": "127.0.0.1:8082",
      "weight": 2,
      "paths": ["/static"]
    }
  ]
}
```

### Config Fields

- `host`: Address to bind the load balancer (default: `0.0.0.0`)
- `listen`: Port to listen on (default: `1902`)
- `algorithm`: Routing algorithm (`round_robin`, `least_conn`, `ip_hash`, `w_round_robin`)
- `max_connections`: Maximum number of active connections
- `timeout_seconds`: Timeout for backend health checks
- `health_check_path`: Path for HTTP health checks
- `health_check_port`: Port for health checks
- `health_check_freq`: Frequency of health checks (in seconds)
- `tls_cert_file` / `tls_key_file`: Paths to TLS certificate and key
- `Backends`: List of backend servers with `address`, `weight`, and optional `paths`

---

## Metrics

Akash provides Prometheus-compatible metrics on `:9100/metrics`.

Exposed metrics include:

- `akash_active_connections` — Number of active client connections
- `akash_backend_served_total{backend="..."}` — Requests successfully served per backend
- `akash_backend_failures_total{backend="..."}` — Failed connections per backend

You can use Grafana to scrape metrics endpoint from Prometheus to build interactive dashboards

---

## Future Plans

Planned features:

- WebSocket support
- HTTP/2 and gRPC proxying
- Retry policies and circuit breakers
- Rate limiting per IP

---

## License

This project is licensed under the [Apache 2.0 License](./LICENSE).
