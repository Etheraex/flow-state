# flow-state

A reverse proxy and load balancer written in Go, built from scratch in phases —
from a plain request forwarder to a resource-aware scheduler with service
discovery and distributed state.

## What it is

`flow-state` sits in front of a pool of backend services. Clients talk to one
stable address; `flow-state` decides where each request actually goes, tracks
which backends are alive and how loaded they are, and sheds load when there's no
capacity left.

It's built as a learning project — the aim is to understand how proxies,
load balancers, and schedulers work by building one, not to compete with nginx
or Envoy.

## Features

- HTTP reverse proxying with streaming bodies and full context cancellation
- Pluggable load-balancing strategies (round-robin, random, weighted,
  least-connections)
- Active and passive health checking with automatic recovery
- Dynamic service discovery — no restart required when backends change
- Shared capacity accounting across multiple proxy instances, backed by Redis
- Resource-aware scheduling with fail-fast backpressure
- Prometheus metrics

## Architecture

```
                    ┌──────────────┐
   clients ────────▶  flow-state   ────────▶ backend pool
                    └──────┬───────┘
                           │
                    ┌──────┴───────┐
                    │    Redis     │  shared capacity state
                    │              │  + service registry
                    └──────────────┘
```

Multiple `flow-state` instances can run in parallel. Because routing decisions
depend on scarce, shared resources, capacity state lives in Redis rather than in
any single instance's memory, and claims are made atomically so two instances
can't hand out the same slot.