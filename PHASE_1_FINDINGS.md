# Phase 1 Findings — httputil reverse proxy

**Setup:** Go `httputil.NewSingleHostReverseProxy`, single backend on `:8081`
(`/work` sleeps 200ms), proxy on `:8080`. Load: 1000 **sequential** requests
per endpoint, localhost, closed-loop. Date: 2026-07-26.

**Metric of interest:** the *delta* between proxied and direct latency — the
shared 200ms backend sleep cancels out, leaving what the proxy itself costs.

---

## Experiment 1 — Baseline overhead (correct client: drain + close body)

**Result:**

| endpoint | p50        | p99        | min        | max        |
|----------|-----------|-----------|-----------|-----------|
| direct   | 201.71ms  | 202.30ms  | 200.65ms  | 204.33ms  |
| proxied  | 202.35ms  | 203.26ms  | 200.93ms  | 205.56ms  |
| **overhead** | **645µs** | **964µs** |          |           |

`ss -tan | grep :8081 | wc -l` → **3**

**Takeaway:** proxy adds sub-millisecond latency. Tight distribution + only 3
sockets = connection pool is reusing connections as intended. p99 overhead >
p50 overhead: extra hop's jitter compounds in the tail (expected shape).

---

## Experiment 2 — What happens without draining/closing the body

Commented out the two lines that let Go reuse the connection:
```go
// io.Copy(io.Discard, resp.Body)
// resp.Body.Close()
```

**Result:**

| endpoint | p50        | p99        | min        | max        |
|----------|-----------|-----------|-----------|-----------|
| direct   | 201.78ms  | 203.78ms  | 200.81ms  | 219.96ms  |
| proxied  | 202.52ms  | 204.32ms  | 201.16ms  | 225.48ms  |
| overhead | 746µs     | 540µs (?) |           |           |

`ss -tan | grep :8081 | wc -l` → **2003**

**Takeaway:** latency barely moved (localhost makes new connections cheap), so
this looks harmless — but the socket count exploded from 3 → 2003: ~one leaked
socket per request. Under real load this ends in `too many open files`.
Note the overhead p99 (540µs) < p50 (746µs): connection-churn noise is not
common-mode, so the delta metric breaks down here — the number is meaningless.

**Conclusion:** drain + close is cheap insurance. Its benefit is invisible on
this benign localhost benchmark but prevents fd exhaustion in production.