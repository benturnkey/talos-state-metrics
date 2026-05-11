package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string        `json:"resultType"`
		Result     []queryResult `json:"result"`
	} `json:"data"`
}

type queryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

func main() {
	prometheusURL := os.Getenv("PROMETHEUS_URL")
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}

	fmt.Printf("Verifying metrics at %s...\n", prometheusURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Timed out waiting for exporter metrics to become valid")
			os.Exit(1)
		case <-ticker.C:
			targetCount, err := healthyTargetCount(prometheusURL)
			if err != nil {
				fmt.Printf("Error checking target health: %v\n", err)
				continue
			}
			if targetCount == 0 {
				fmt.Println("Waiting for talos-state-metrics targets to come online...")
				continue
			}

			fmt.Printf("All %d talos-state-metrics targets are UP.\n", targetCount)
			ok, err := checkMetrics(prometheusURL, targetCount)
			if err != nil {
				fmt.Printf("Error checking metrics: %v\n", err)
				continue
			}
			if ok {
				fmt.Println("Verified: exporter self-metrics and peer-derived metrics are present in Prometheus!")
				os.Exit(0)
			}
			fmt.Println("Waiting for Prometheus to scrape a full set of exporter metrics...")
		}
	}
}

func healthyTargetCount(baseURL string) (int, error) {
	upResults, err := instantVector(baseURL, `up{namespace="monitoring",pod=~"talos-state-metrics-.*"}`)
	if err != nil {
		return 0, err
	}
	if len(upResults) == 0 {
		return 0, nil
	}

	for _, sample := range upResults {
		value, err := sampleValue(sample)
		if err != nil {
			return 0, err
		}
		if value != 1 {
			return 0, nil
		}
	}

	return len(upResults), nil
}

func checkMetrics(baseURL string, targetCount int) (bool, error) {
	watchConnected, err := instantVector(baseURL, "talos_state_metrics_watch_connected")
	if err != nil {
		return false, err
	}
	if len(watchConnected) != targetCount {
		return false, nil
	}
	for _, sample := range watchConnected {
		value, err := sampleValue(sample)
		if err != nil {
			return false, err
		}
		if value != 1 {
			return false, nil
		}
	}

	peerCounts, err := instantVector(baseURL, "talos_kubespan_peer_count")
	if err != nil {
		return false, err
	}
	if len(peerCounts) != targetCount {
		return false, nil
	}

	return true, nil
}

func instantVector(baseURL, query string) ([]queryResult, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", baseURL, url.QueryEscape(query))
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var qr queryResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, err
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("prometheus query for %q returned status %q", query, qr.Status)
	}
	if qr.Data.ResultType != "vector" {
		return nil, fmt.Errorf("prometheus query for %q returned resultType %q", query, qr.Data.ResultType)
	}

	return qr.Data.Result, nil
}

func sampleValue(sample queryResult) (float64, error) {
	if len(sample.Value) != 2 {
		return 0, fmt.Errorf("unexpected sample value shape: %#v", sample.Value)
	}

	valueText, ok := sample.Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected sample value type: %#v", sample.Value[1])
	}

	return strconv.ParseFloat(valueText, 64)
}
