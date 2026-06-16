# Runbook: High API Error Rate

## Alert

ReliabilityOpsHighErrorRate

## Symptoms

- Elevated 5xx responses
- Users may see failed API requests
- Grafana shows increased error rate

## Quick Checks

```bash
curl -s http://localhost:8080/ready
curl -s http://localhost:8080/metrics | grep reliabilityops_http_requests_total
docker compose logs api --tail=100
