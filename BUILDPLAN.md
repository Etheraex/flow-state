# flow-state

A reverse proxy in Go, built in phases — from a dumb request forwarder to a
resource-aware scheduler with distributed state and service discovery.

This is a learning project. The goal is not to build a better nginx; it is to
arrive at each piece of machinery *because the previous phase made its absence
painful*. Every phase exists to surface a specific failure mode, and the next
phase exists to fix it.

---

## Stack

| Concern | Choice |
| --- | --- |
| Language | Go |
| HTTP | `net/http`, `net/http/httputil` |
| Shared state | Redis (from Phase 6) |
| Metrics | Prometheus (`/metrics` endpoint) |
| Load generation | `vegeta` or `k6` |
| Test backends | Trivial Go services with configurable latency, error rate, and capacity |

Get the load generator and mock backends in place at **Phase 2**, not later.
Most lessons from Phase 4 onward are invisible without concurrent traffic.

Run everything under the **race detector** from Phase 2 on: `go run -race` while
load testing, `go test -race` in CI. Every phase from here adds shared mutable state
(the rotating index, the live-backend set, connection counters), and `-race` surfaces
the exact data race each phase is built to teach — with both goroutines' stacks. Treat
"passes `go test -race` under concurrent load" as an implicit Done-when for Phases 2–7.

---

## Phases

### Phase 1 — Plain reverse proxy

**Goal:** forward requests to a single hardcoded backend and return the response.

Build it twice. First with `httputil.ReverseProxy` to see the shape. Then
hand-roll the forwarding loop — that second version is where the real learning
is, because the stdlib quietly handles things you need to understand.

**Concepts:**
- Hop-by-hop vs. end-to-end headers (`Connection`, `Transfer-Encoding`, `TE`)
- `X-Forwarded-For` / `X-Forwarded-Proto` / `Forwarded`
- Streaming the body rather than buffering it into memory
- `context.Context` propagation — a client disconnect must tear down the
  upstream call, not orphan it
- Timeouts at every layer: dial, response header, idle, total
- **Graceful shutdown.** Trap SIGTERM/SIGINT and call `http.Server.Shutdown` to
  drain in-flight requests before exiting, rather than cutting them off. Server
  lifecycle is part of the proxy, not an afterthought — and it's the *clean* half of
  a contrast you'll complete in Phase 6.
- A propagated **request ID**: generate one if the client didn't send it, echo it
  in a response header, and log it. This is the correlation key you'll need to
  trace a single request across proxy and backend once concurrency (Phase 5+)
  makes logs interleave. Cheap now, invaluable later.

**Done when:** a client disconnect mid-response visibly cancels the upstream
request, you can explain which headers you strip and why, **and you have measured
the proxy's added latency against hitting the backend directly (p50/p99) — this is
the baseline overhead number every later phase is compared against.**

---

### Phase 2 — Multiple backends, dumb load balancing

**Goal:** a static pool of backends, with round-robin and random selection.

**Concepts:**
- A `Balancer` interface — `Pick(*http.Request) (*Backend, error)`. Define this
  now; every later phase becomes a new implementation instead of a rewrite.
- `sync/atomic` vs. `sync.Mutex` for the rotating index. Every concurrent
  request touches it, so this is the first real contention point.
- Weighted round-robin as a variant.
- **Backend connection reuse.** The forwarding path talks to backends through an
  `http.Transport` with a connection pool. Defaults (`MaxIdleConns`,
  `MaxIdleConnsPerHost`, keep-alives) quietly decide whether you reopen a TCP+TLS
  connection per request or reuse a warm one — a large, easily-missed chunk of the
  overhead you baselined in Phase 1. Tune it deliberately and re-measure; "why is my
  proxy slow under load" is very often "per-host idle connections is 2."

**Done when:** load generation shows an even distribution across backends, you can
swap strategies via config without touching the proxy core, and it's clean under
`go test -race` — the rotating index is your first shared-state race, so prove it.

---

### Phase 3 — Health checking

**Goal:** stop routing to backends that are down.

**Concepts:**
- Active checks: a background goroutine on a `time.Ticker` probing each backend
- Passive checks: mark a backend suspect on connection failure or 5xx
- `sync.RWMutex` around the live set — one writer, thousands of readers
- Recovery semantics: how many consecutive successes before a backend is
  considered healthy again? (Flapping is the failure mode here.)
- Circuit-breaker thinking: fail fast on a known-bad backend rather than
  paying the timeout every request

**Done when:** killing a backend mid-load-test causes a brief error blip that
self-heals, and restarting it brings it back into rotation automatically.

**This phase also introduces the question that drives Phase 3.5:** how stale is
your view of the world, and where does the backend list come from at all?

---

### Phase 3.5 — Service discovery

The backend list has been hardcoded until now. That is the one assumption real
deployments never satisfy: container IPs are ephemeral, autoscaling changes
membership continuously, and rolling deploys replace instances one at a time.

Note what you're building: clients know only about **one** stable address (the
proxy), and the proxy owns the question of who is actually behind it. That is
**server-side discovery**, as opposed to *client-side discovery* where every
caller queries the registry and load-balances itself.

Service discovery decomposes into three parts worth keeping separate:

1. **Registration** — how an instance becomes known.
   *Self-registration* (the instance announces itself) vs. *third-party
   registration* (something else does it — e.g. the Kubernetes control plane
   watching pods).
2. **The registry** — the store of current membership (Consul, etcd, ZooKeeper,
   Eureka, the K8s API server).
3. **Resolution** — how the consumer gets the list.

Build it incrementally:

**3a — Hot-reloadable config.** Watch the config file; rebuild the backend set
without a restart. No registry yet, but it introduces the hard part (see below).

**3b — DNS-based discovery.** Resolve a hostname on a timer and take the A
records as your pool. This is how a surprising amount of production discovery
still works — and you'll immediately hit its limits: no health awareness, TTL
caching that lies to you, and no port information (that's what SRV records are
for).

**3c — Registry-backed, using Redis.** Backends self-register under a key with a
TTL and refresh it on a heartbeat; the proxy reads the live keys. Dead instances
simply expire. You will have built a real, if minimal, registry.

**3d — Optional: a real registry.** Point it at Consul or etcd, and switch from
*polling* to *watching* — a long-lived subscription that pushes changes rather
than you asking every few seconds. Bounded staleness vs. near-immediate
propagation.

**Concepts:**
- **Registration is a lease.** Instances renew; entries expire if they stop.
  This is the same mechanism as the capacity leases in Phase 6 — the general
  answer to "how does a distributed system reclaim state from something that
  stopped responding."
- **Registered ≠ healthy.** The registry says who *exists*; health checks say
  who is *usable*. The routable set is the intersection.
- **The consistency tradeoff.** etcd/ZooKeeper/Consul are consistency-favoring
  (Raft) — they'd rather refuse to serve than serve wrong membership during a
  partition. Eureka chose availability — serve a possibly-stale list, because
  routing to a dead instance is recoverable (health check catches it, retry
  elsewhere) while having *no* list is a total outage. Form an opinion.

**The part that's actually hard:**
- **Swapping the live set under concurrent reads.** A `sync.RWMutex` around a
  slice works, but on a hot path the better idiom is to build the new set
  immutably and swap the pointer with `atomic.Pointer[T]` — readers get a
  consistent snapshot, no lock contention, writers never block them.
- **In-flight requests to a vanished instance.** Stop routing *new* work there
  while letting existing work finish: connection draining.
- **Registry blips.** If the registry momentarily reports zero instances and the
  proxy dutifully rejects everything, that's a real outage pattern. Keep the
  last known good set; refuse to accept an empty membership update.

**Done when:** starting and stopping backend instances changes the routing pool
with no proxy restart, a killed instance ages out via TTL expiry, and a
deliberately broken registry does *not* take the proxy down.

---

### Phase 3.6 — Retries, timeouts, and idempotency (partial failure)

**Goal:** survive a *single request* failing without failing the client — and learn
exactly when you're allowed to.

Health checking (Phase 3) routes around backends you *know* are down. But your view
is always stale — the closing question of Phase 3. A backend can pass its last probe
and still reset the connection you open right now, or die mid-response. The fix is to
retry on a different backend. Retries are also a loaded gun.

**Concepts:**
- **Idempotency decides eligibility.** GET/PUT/DELETE can be safely re-sent; a POST
  may not (double-charge, double-write). Retry only what's safe, or demand an
  idempotency key. This is a correctness question, not a performance one.
- **What to retry on.** Connection/dial failures — almost always safe. A 5xx — maybe.
  A *timeout* — dangerous: the first attempt may still be running on the backend, so a
  retry duplicates in-flight work.
- **Retry budgets, not unlimited retries.** Naive per-request retries amplify load:
  when backends struggle, every client retries, traffic multiplies, and a brownout
  becomes an outage. Cap per-request retries (1–2) *and* enforce a global retry budget
  (retries as a fraction of total traffic). This is the mechanism that prevents
  **retry-storm cascading failure** — and the conceptual sibling of Phase 7's load
  shedding.
- **Jitter and backoff.** Synchronized retries are a thundering herd; randomize.
- **Timeout budget across hops.** The proxy's total deadline must be *shorter* than
  the client's, and each retry spends part of it — no budget left, no retry. This is
  `context` deadline propagation from Phase 1 doing real work.
- **Interaction with the balancer:** a retry must pick a *different* backend, and —
  foreshadowing Phase 4 — must correctly release and re-claim per-backend accounting.

**Done when:** killing a backend *mid-request* (not just leaving it idle) produces a
transparent retry to a healthy backend for an idempotent request; a non-idempotent
request is *not* silently retried; and you can show that a backend brownout is not
amplified into a traffic multiplier by your retry logic.

---

### Phase 4 — Least-connections

**Goal:** route to the backend with the fewest in-flight requests.

This is the pivotal phase. Least-connections requires **claim-and-release
accounting**: increment a per-backend counter on dispatch, `defer` the decrement
on completion. In a single process this is easy — which is exactly why it's the
right place to learn the semantics.

**Concepts:**
- Claim/release as a pattern
- The **leak bug**: miss the decrement on a panic or timeout path and that
  backend's count drifts upward forever until it is never selected again. Cause
  this deliberately, watch it happen, then fix it.
- Why `defer` is not sufficient on its own if the request outlives the handler
- **Admission control falls out for free here.** Once you're counting in-flight
  requests, you can reject when *every* backend is at its cap instead of dispatching
  into an overloaded pool — a global max-in-flight limit. This is the simple,
  single-process ancestor of Phase 7's resource-aware load shedding: same instinct
  (fail fast rather than queue), one phase earlier, on a counter you already maintain.
  Return a clean 503 with a retry hint, and make the rejection a metric.

**Done when:** you have reproduced a counter leak, and your metrics would have
told you it was happening.

---

### Phase 5 — Scale the proxy horizontally, and watch it break

**Goal:** run *two* instances of the Phase 4 proxy in front of the same backends.

Do not add Redis yet. Each proxy has its own in-memory connection counts, so
both will independently conclude that backend 3 is least-loaded, and both will
pile onto it.

**Done when:** you can *show* the oversubscription on a graph. This phase has no
deliverable except a demonstrated problem — which is what earns the next one.

---

### Phase 6 — Externalize state to Redis, then fix the race

**Goal:** shared capacity accounting across proxy instances.

Implement it **naively first**: `GET` the counts, pick the minimum, `DECR`.
Then drive enough concurrent load to actually produce the failure.

**The check-then-act race:** proxy A reads "backend 3 has 1 slot free," proxy B
reads the same, both decide to route there, both decrement. Now the count is
negative and you have promised capacity that doesn't exist.

Fix it in this order, one commit each:

1. **Lua script for atomic select-and-claim.** Redis is single-threaded and
   executes a script atomically — nothing interleaves. Push the whole
   "find least-loaded → verify under capacity → claim → return the choice"
   sequence into one script. ~15 lines.
   Note: scripts are *invoked explicitly* (`EVAL` / `EVALSHA`), not triggered by
   events. The right SQL analogy is a stored procedure, not a trigger.
   Keys must be passed via `KEYS[]`, never constructed inside the script, or you
   break under Redis Cluster. Keep scripts short — a slow script stalls every
   other client.
2. **TTL'd leases.** A claim that is never released must expire on its own, or a
   crashed proxy leaks capacity permanently.
3. **Heartbeat reconciliation.** Backends report their true available capacity
   periodically; use it to correct Redis drift. Redis is a fast-routing
   optimization, not the sole authority.

**Concepts:**
- This is a **distributed semaphore**. If your instinct reached for
  `SemaphoreSlim`, that instinct is correct — Redis holds the permit count, the
  atomic decrement is `Wait()`, the increment is `Release()`.
- Redis is now a hot-path dependency. Contain the blast radius: a Redis hiccup
  should degrade routing *quality*, not availability.

**Done when:** two proxy instances under load never oversubscribe a backend, a
`kill -9` on one proxy returns its held capacity within the lease TTL, and
stopping Redis degrades rather than kills the system.
Contrast the two shutdown paths explicitly: a **graceful** stop (SIGTERM →
`Shutdown`) should drain in-flight work and return its held capacity to Redis
*immediately*, while a `kill -9` can only recover via the lease TTL. Feeling that gap
is the point — it's the concrete reason leases exist: they cover the termination you
*can't* make graceful.

---

### Phase 7 — Resource-aware scheduling

**Goal:** generalize from "connection count" to "capacity for a specific
resource".

**Concepts:**
- Backends declare capacity per resource type
- One Redis **sorted set (ZSET)** per resource: members are backends, score is
  available capacity. `ZRANGE ... REV 0 0` gives the most-available backend in
  O(log N).
- **Backpressure by rejection.** When nothing has capacity, reject immediately
  rather than queueing. Fail-fast load shedding turns resource exhaustion into a
  clean, fast signal instead of a slow-motion latency collapse that hides the
  overload from everyone.
- Cluster/pool partitioning: dedicated pools for latency-sensitive workloads,
  shared pool for everything else, to avoid noisy-neighbour effects.

**Done when:** a saturated resource produces fast, explicit rejections with a
sane retry signal — and the rejection rate is a metric you can alert on.

---

## Observability

Instrument from Phase 3 onward, not at the end:

- Picks per backend (distribution fairness)
- In-flight per backend
- Rejections, by reason
- Health state transitions
- Registry update frequency, and membership set size
- Redis call latency and error rate
- End-to-end proxy overhead vs. direct-to-backend latency
- Latencies as **percentiles (p50/p99), not averages** — tail latency is the whole
  point of scheduling; an average hides the backend that's occasionally slow.
- A **request ID on every log line** (from Phase 1), so one request is traceable
  across proxy → backend. Graduate to OpenTelemetry spans if you want real
  distributed tracing (optional, orthogonal — like TLS, save it for later).

The last one matters: the point of comparison for any layer you insert is
whether it adds meaningful overhead over talking to the backend directly.

---

## Non-goals

- Production readiness
- TLS termination, HTTP/2 or HTTP/3 support, WebSocket upgrades
  (interesting, but orthogonal to the scheduling thread — save for later)
- Beating existing proxies on any benchmark

---

## Reference reading

- **GitFarm: Git as a Service for Large-Scale Monorepos** (Uber) — the
  gateway/Redis/pooling design this project's later phases are modelled on
- **Zero-Growth Stack, Real Gains: How Stack Allocation Can Save 10% CPU in Go**
  (Uber) — runtime-level performance reasoning
- **CRISP: Critical Path Analysis for Microservice Architectures** (Uber)
- Redis docs: scripting (`EVAL`, Redis Functions), sorted sets, key expiry
- Go: `net/http/httputil`, `sync/atomic`, `context`

---

## Naming

`flow-state` — traffic flow, and the state required to route it well.