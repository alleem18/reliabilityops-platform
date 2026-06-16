# Runbook: High API Latency

## Alert

ReliabilityOpsHighLatencyP95

## Symptoms

- p95 latency above 1 second
- Slow API responses
- Grafana latency panel spikes

## Quick Checks

```bash
curl -s http://localhost:8080/ready
docker compose stats
docker compose logs api --tail=100
