package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	Repos          []string `json:"repos"`
	IP             string   `json:"ip"`
	Port           int      `json:"port"`
	Endpoint       string   `json:"endpoint"`
	TickerInterval int      `json:"ticker_interval"`
}

const (
	defaultIP             = "0.0.0.0"
	defaultPort           = 8080
	defaultEndpoint       = "/metrics"
	defaultTickerInterval = 60 // in seconds
)

var (
	lastTransactionTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "borgbackup_last_transaction_timestamp",
			Help: "Unix timestamp of the last transaction in the BorgBackup repository",
		},
		[]string{"repo"},
	)
	lastTransactionNumber = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "borgbackup_last_transaction_number",
			Help: "Number of the last transaction in the BorgBackup repository",
		},
		[]string{"repo"},
	)
)

func init() {
	prometheus.MustRegister(lastTransactionTimestamp)
	prometheus.MustRegister(lastTransactionNumber)
}

func main() {
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	applyDefaults(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		updateMetricsLoop(ctx, config.Repos, time.Duration(config.TickerInterval)*time.Second)
	}()

	serverAddr := fmt.Sprintf("%s:%d", config.IP, config.Port)
	http.Handle(config.Endpoint, promhttp.Handler())
	server := &http.Server{Addr: serverAddr}

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Starting Prometheus exporter on %s%s\n", serverAddr, config.Endpoint)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	<-sigChan
	log.Println("Received termination signal. Shutting down...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}

	wg.Wait()
	log.Println("Exporter stopped.")
}

func loadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func applyDefaults(config *Config) {
	if config.IP == "" {
		config.IP = defaultIP
	}
	if config.Port == 0 {
		config.Port = defaultPort
	}
	if config.Endpoint == "" {
		config.Endpoint = defaultEndpoint
	}
	if config.TickerInterval == 0 {
		config.TickerInterval = defaultTickerInterval
	}
}

func updateMetricsLoop(ctx context.Context, repos []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

  // Perform the first update immediately
	for _, repo := range repos {
		updateRepoMetrics(repo)
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping metrics update loop.")
			return
		case <-ticker.C:
			for _, repo := range repos {
				updateRepoMetrics(repo)
			}
		}
	}
}

func updateRepoMetrics(repo string) {
	transactionsFile := filepath.Join(repo, "transactions")
	file, err := os.Open(transactionsFile)
	if err != nil {
		log.Printf("Failed to open transactions file for repo %s: %v", repo, err)
		return
	}
	defer file.Close()

	var lastLine string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lastLine = scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading transactions file for repo %s: %v", repo, err)
		return
	}

  // instead of deferring as usual, close as soon as the 
  file.Close()

	transactionNumber, timestamp, err := parseTransactionLine(lastLine)
	if err != nil {
		log.Printf("Failed to parse transactions file for repo %s: %v", repo, err)
		return
	}

	lastTransactionTimestamp.WithLabelValues(repo).Set(float64(timestamp))
	lastTransactionNumber.WithLabelValues(repo).Set(float64(transactionNumber))
}

func parseTransactionLine(line string) (int, int64, error) {
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid line format: %s", line)
	}

	numberStr := strings.TrimPrefix(parts[0], "transaction ")
	transactionNumber, err := strconv.Atoi(strings.TrimSpace(numberStr))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse transaction number: %v", err)
	}

	timeStr := strings.TrimSpace(strings.Replace(parts[1], "UTC time", "", -1))
	t, err := time.Parse("2006-01-02T15:04:05.000000", timeStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse UTC time: %v", err)
	}

	return transactionNumber, t.Unix(), nil
}
