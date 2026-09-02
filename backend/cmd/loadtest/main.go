// Command loadtest measures what the brief actually asks to be proven: that
// the server sustains N simultaneous listeners on one live stream "avec une
// consommation mémoire minimale".
//
// The end-to-end test in the router package proves *correctness* at 25
// listeners — every listener receives every chunk. That is a different
// question from this one. Correctness says nothing about the latency to first
// audio under load, about whether a listener falls behind the broadcaster, or
// about how much memory each additional listener costs. Those are the numbers
// a jury asks for, and the ones that decide whether the pub/sub design holds.
//
// Deliberately a Go program rather than a k6 script: the thing under test is a
// long-lived chunked HTTP body, and a generic HTTP load tool measures
// request/response round trips. Here a "request" never ends — what matters is
// the byte stream inside it. Go also lets the harness read the server's own
// /metrics, so memory is measured from the process under test rather than
// guessed from the outside.
//
//	go run ./cmd/loadtest -listeners 200 -duration 30s
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

type config struct {
	api       string
	listeners int
	duration  time.Duration
	bitrate   int
	rampUp    time.Duration
}

func run() error {
	var cfg config
	flag.StringVar(&cfg.api, "api", "http://localhost:8080", "base URL of the API")
	flag.IntVar(&cfg.listeners, "listeners", 100, "number of simultaneous listeners")
	flag.DurationVar(&cfg.duration, "duration", 30*time.Second, "how long listeners stay connected")
	flag.IntVar(&cfg.bitrate, "bitrate", 128, "broadcast bitrate in kbit/s")
	flag.DurationVar(&cfg.rampUp, "ramp-up", 2*time.Second, "spread listener connections over this window")
	flag.Parse()

	if cfg.listeners < 1 {
		return fmt.Errorf("-listeners must be at least 1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration+2*time.Minute)
	defer cancel()

	fmt.Printf("StreamPulse load test\n  api=%s listeners=%d duration=%s bitrate=%dkbps\n\n",
		cfg.api, cfg.listeners, cfg.duration, cfg.bitrate)

	token, err := signUpBroadcaster(ctx, cfg.api)
	if err != nil {
		return fmt.Errorf("sign up broadcaster: %w", err)
	}
	streamID, err := createStream(ctx, cfg.api, token)
	if err != nil {
		return fmt.Errorf("create stream: %w", err)
	}
	fmt.Printf("stream %s created\n", streamID)

	before := readRuntimeMetrics(ctx, cfg.api)

	// The broadcaster runs for the whole test; listeners attach to it.
	broadcastCtx, stopBroadcast := context.WithCancel(ctx)
	defer stopBroadcast()
	broadcastDone := make(chan error, 1)
	go func() { broadcastDone <- broadcast(broadcastCtx, cfg, token, streamID) }()

	// Wait for the stream to actually go live before connecting listeners: a
	// listener that arrives first would be measuring the broadcaster's startup,
	// not the server's fan-out.
	if err := waitLive(ctx, cfg.api, streamID); err != nil {
		return fmt.Errorf("stream never went live: %w", err)
	}

	// Sample while the listeners are actually connected. Reading /metrics
	// after runListeners returns would measure the server at rest — the first
	// version of this harness did exactly that and reported 20 goroutines for
	// 50 listeners, a number that cannot be true and that made the memory
	// figures meaningless.
	sampleCtx, stopSampling := context.WithCancel(ctx)
	peakCh := make(chan runtimeSample, 1)
	go func() { peakCh <- samplePeak(sampleCtx, cfg.api) }()

	results := runListeners(ctx, cfg, streamID)

	stopSampling()
	during := <-peakCh
	stopBroadcast()
	<-broadcastDone
	// Give the server a moment to release the closed subscriptions before
	// reading the "after" sample, otherwise it measures teardown in flight.
	time.Sleep(2 * time.Second)
	after := readRuntimeMetrics(ctx, cfg.api)

	report(cfg, results, before, during, after)
	return nil
}

// --- API helpers -----------------------------------------------------------

func signUpBroadcaster(ctx context.Context, api string) (string, error) {
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	body := map[string]string{
		"email":    "loadtest-" + suffix + "@test.local",
		"username": "loadtest-" + suffix,
		"password": "load-test-password-123",
	}

	var auth struct {
		Token string `json:"token"`
	}
	if err := postJSON(ctx, api+"/api/v1/auth/register", "", body, &auth); err != nil {
		return "", err
	}
	return auth.Token, nil
}

func createStream(ctx context.Context, api, token string) (string, error) {
	var stream struct {
		ID string `json:"id"`
	}
	body := map[string]string{"title": "Load test", "description": "synthetic broadcast"}
	if err := postJSON(ctx, api+"/api/v1/streams", token, body, &stream); err != nil {
		return "", err
	}
	return stream.ID, nil
}

func waitLive(ctx context.Context, api, streamID string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api+"/api/v1/streams/"+streamID, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			var s struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&s)
			_ = resp.Body.Close()
			if s.Status == "live" {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for status=live")
}

// broadcast streams synthetic audio at the configured bitrate until ctx ends.
//
// The payload is random bytes, not a real container: the server multiplexes
// opaque chunks and never parses them, so encoding real audio would measure
// the encoder rather than the hub.
func broadcast(ctx context.Context, cfg config, token, streamID string) error {
	const chunkInterval = 100 * time.Millisecond
	chunkSize := cfg.bitrate * 1000 / 8 / int(time.Second/chunkInterval)

	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.api+"/api/v1/streams/"+streamID+"/publish", pr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "audio/mpeg")

	done := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- err
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		done <- nil
	}()

	chunk := make([]byte, chunkSize)
	_, _ = rand.Read(chunk)
	ticker := time.NewTicker(chunkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = pw.Close()
			<-done
			return nil
		case <-ticker.C:
			if _, err := pw.Write(chunk); err != nil {
				_ = pw.Close()
				<-done
				return nil
			}
		}
	}
}

// --- Listeners -------------------------------------------------------------

type listenerResult struct {
	connected bool
	status    int
	ttfb      time.Duration
	bytes     int64
	err       error
}

func runListeners(ctx context.Context, cfg config, streamID string) []listenerResult {
	results := make([]listenerResult, cfg.listeners)
	var wg sync.WaitGroup

	// Spread connections over the ramp-up window: opening N sockets in the
	// same microsecond measures the kernel's accept queue, not the hub.
	gap := time.Duration(0)
	if cfg.listeners > 1 {
		gap = cfg.rampUp / time.Duration(cfg.listeners-1)
	}

	start := time.Now()
	for i := 0; i < cfg.listeners; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Duration(i) * gap)
			results[i] = listen(ctx, cfg, streamID, start.Add(cfg.rampUp+cfg.duration))
		}(i)
	}
	wg.Wait()
	return results
}

func listen(ctx context.Context, cfg config, streamID string, until time.Time) listenerResult {
	var res listenerResult

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.api+"/api/v1/streams/"+streamID+"/listen", nil)
	if err != nil {
		res.err = err
		return res
	}

	begin := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		res.err = err
		return res
	}
	defer func() { _ = resp.Body.Close() }()

	res.status = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		res.err = fmt.Errorf("status %d", resp.StatusCode)
		return res
	}
	res.connected = true

	buf := make([]byte, 32*1024)
	first := true
	for time.Now().Before(until) {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if first {
				// Time to first audio byte: what a real listener experiences
				// as "how long before I hear anything".
				res.ttfb = time.Since(begin)
				first = false
			}
			res.bytes += int64(n)
		}
		if err != nil {
			if err != io.EOF {
				res.err = err
			}
			break
		}
	}
	return res
}

// --- Server-side metrics ---------------------------------------------------

type runtimeSample struct {
	available  bool
	heapBytes  float64
	goroutines float64
}

// samplePeak polls /metrics until ctx ends and keeps the highest reading of
// each series. Peak rather than average: the question is what the server needs
// at its worst moment, since that is what has to fit in the machine.
func samplePeak(ctx context.Context, api string) runtimeSample {
	var peak runtimeSample

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return peak
		case <-ticker.C:
			s := readRuntimeMetrics(ctx, api)
			if !s.available {
				continue
			}
			peak.available = true
			if s.heapBytes > peak.heapBytes {
				peak.heapBytes = s.heapBytes
			}
			if s.goroutines > peak.goroutines {
				peak.goroutines = s.goroutines
			}
		}
	}
}

// readRuntimeMetrics scrapes the server's own /metrics. Absent endpoint is not
// an error: the endpoint ships with the observability ticket, and the load
// test must stay runnable against a build without it — it then reports client
// numbers only and says so, rather than pretending memory was measured.
func readRuntimeMetrics(ctx context.Context, api string) runtimeSample {
	var s runtimeSample

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/metrics", nil)
	if err != nil {
		return s
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return s
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s
	}
	for _, line := range strings.Split(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "go_memstats_heap_alloc_bytes "):
			s.heapBytes, _ = strconv.ParseFloat(strings.Fields(line)[1], 64)
			s.available = true
		case strings.HasPrefix(line, "go_goroutines "):
			s.goroutines, _ = strconv.ParseFloat(strings.Fields(line)[1], 64)
			s.available = true
		}
	}
	return s
}

// --- Report ----------------------------------------------------------------

func report(cfg config, results []listenerResult, before, during, after runtimeSample) {
	var connected, failed int
	var totalBytes int64
	ttfbs := make([]time.Duration, 0, len(results))
	byteCounts := make([]int64, 0, len(results))

	for _, r := range results {
		if !r.connected {
			failed++
			continue
		}
		connected++
		totalBytes += r.bytes
		byteCounts = append(byteCounts, r.bytes)
		if r.ttfb > 0 {
			ttfbs = append(ttfbs, r.ttfb)
		}
	}

	fmt.Printf("\n=== Listeners ===\n")
	fmt.Printf("  connected            %d / %d\n", connected, cfg.listeners)
	fmt.Printf("  failed to connect    %d\n", failed)

	if len(ttfbs) > 0 {
		sort.Slice(ttfbs, func(i, j int) bool { return ttfbs[i] < ttfbs[j] })
		fmt.Printf("\n=== Time to first audio byte ===\n")
		fmt.Printf("  p50                  %s\n", percentileDur(ttfbs, 0.50).Round(time.Millisecond))
		fmt.Printf("  p95                  %s\n", percentileDur(ttfbs, 0.95).Round(time.Millisecond))
		fmt.Printf("  p99                  %s\n", percentileDur(ttfbs, 0.99).Round(time.Millisecond))
		fmt.Printf("  max                  %s\n", ttfbs[len(ttfbs)-1].Round(time.Millisecond))
	}

	if len(byteCounts) > 0 {
		sort.Slice(byteCounts, func(i, j int) bool { return byteCounts[i] < byteCounts[j] })
		expected := int64(cfg.bitrate) * 1000 / 8 * int64(cfg.duration/time.Second)
		median := byteCounts[len(byteCounts)/2]
		worst := byteCounts[0]

		fmt.Printf("\n=== Throughput ===\n")
		fmt.Printf("  fanned out total     %.1f MiB\n", float64(totalBytes)/(1<<20))
		fmt.Printf("  expected / listener  ~%.1f KiB\n", float64(expected)/1024)
		fmt.Printf("  median / listener    %.1f KiB (%.0f%% of expected)\n",
			float64(median)/1024, 100*float64(median)/float64(expected))
		// The slowest listener is the interesting one: the design claims a
		// slow subscriber cannot be starved by its peers.
		fmt.Printf("  slowest listener     %.1f KiB (%.0f%% of expected)\n",
			float64(worst)/1024, 100*float64(worst)/float64(expected))
	}

	fmt.Printf("\n=== Server memory ===\n")
	if !before.available {
		fmt.Printf("  /metrics unavailable on this build — client-side numbers only\n")
		return
	}
	fmt.Printf("  heap before          %.1f MiB (%.0f goroutines)\n", before.heapBytes/(1<<20), before.goroutines)
	fmt.Printf("  heap at peak load    %.1f MiB (%.0f goroutines)\n", during.heapBytes/(1<<20), during.goroutines)
	fmt.Printf("  heap after           %.1f MiB (%.0f goroutines)\n", after.heapBytes/(1<<20), after.goroutines)
	if connected > 0 {
		delta := during.heapBytes - before.heapBytes
		fmt.Printf("  cost per listener    %.1f KiB\n", delta/float64(connected)/1024)
	}
	// Goroutines returning to their baseline is the leak check: every
	// listener's subscription goroutine must end when its connection does.
	fmt.Printf("  goroutine delta      %+.0f (want ~0: a leak shows up here)\n",
		after.goroutines-before.goroutines)
}

func percentileDur(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p * float64(len(sorted)-1))
	return sorted[i]
}

func postJSON(ctx context.Context, url, token string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s -> %d: %s", url, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
