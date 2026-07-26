package main

import (
	"io"
	"log"
	"math"
	"net/http"
	"slices"
	"time"
)

const (
	numberOfRequests = 1000
	directURL        = "http://localhost:8081/work"
	proxyURL         = "http://localhost:8080/work"
)

type stats struct {
	n        int
	failures int
	p50, p99 time.Duration
	min, max time.Duration
}

func measure(url string, n int) ([]time.Duration, int) {
	durations := make([]time.Duration, 0, n)
	failures := 0

	for i := 0; i < n; i++ {
		start := time.Now()

		resp, err := http.Get(url)

		if err != nil {
			failures++
			continue
		}

		// fully drain the body and close it, this is what lets Go return
		// the TCP connection to the pool and reuse it next iteration
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		durations = append(durations, time.Since(start))
	}

	return durations, failures
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))

	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func summarize(durations []time.Duration, failures int) stats {
	s := stats{n: len(durations), failures: failures}

	if len(durations) == 0 {
		return s
	}

	slices.Sort(durations)

	s.min = durations[0]
	s.max = durations[len(durations)-1]
	s.p50 = percentile(durations, 50)
	s.p99 = percentile(durations, 99)

	return s
}

func report(name string, s stats) {
	if s.n == 0 {
		log.Printf("%-8s no successful requests (%d failures)\n", name, s.failures)
		return
	}
	log.Printf("%-8s n=%-5d failures=%-3d p50=%-8v p99=%-8v min=%-8v max=%-8v ",
		name, s.n, s.failures, s.p50, s.p99, s.min, s.max)
}

func main() {
	log.Printf("firing %d sequential requests at each endpoint... \n\n", numberOfRequests)

	direct := summarize(measure(directURL, numberOfRequests))
	proxied := summarize(measure(proxyURL, numberOfRequests))

	report("direct", direct)
	report("proxied", proxied)

	log.Printf("\noverhead p50=%v p99=%v\n",
		proxied.p50-direct.p50, proxied.p99-direct.p99)
}
