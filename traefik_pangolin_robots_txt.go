// Package traefik_pangolin_robots_txt a plugin to complete robots.txt file.
package traefik_pangolin_robots_txt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config the plugin configuration.
type Config struct {
	// CustomRules contains custom robots.txt rules to append
	CustomRules string `json:"customRules,omitempty"`
	
	// Overwrite determines if original robots.txt content should be replaced
	Overwrite bool `json:"overwrite,omitempty"`
	
	// AiRobotsTxt enables fetching AI robots.txt rules from external source
	AiRobotsTxt bool `json:"aiRobotsTxt,omitempty"`
	
	// LastModified controls whether to preserve Last-Modified headers
	LastModified bool `json:"lastModified,omitempty"`
	
	// AiRobotsTxtURL allows custom URL for AI robots.txt source
	// Defaults to GitHub repository if not specified
	AiRobotsTxtURL string `json:"aiRobotsTxtUrl,omitempty"`
	
	// AiRobotsTxtPath allows specifying a local file path instead of URL
	// Takes precedence over AiRobotsTxtURL if both are specified
	AiRobotsTxtPath string `json:"aiRobotsTxtPath,omitempty"`
	
	// CacheTTL specifies how long to cache external content (in seconds)
	// Default: 300 seconds (5 minutes)
	CacheTTL int `json:"cacheTtl,omitempty"`
	
	// MaxRetries specifies maximum retry attempts for external requests
	// Default: 3
	MaxRetries int `json:"maxRetries,omitempty"`
	
	// RequestTimeout specifies timeout for external HTTP requests (in seconds)
	// Default: 10 seconds
	RequestTimeout int `json:"requestTimeout,omitempty"`
	
	// FallbackContent provides content to use when external sources fail
	FallbackContent string `json:"fallbackContent,omitempty"`
	
	// EnableMetrics enables detailed logging for monitoring
	EnableMetrics bool `json:"enableMetrics,omitempty"`
}

// cacheEntry represents a cached robots.txt content with expiration
type cacheEntry struct {
	content   string
	expiresAt time.Time
	source    string // URL or file path for debugging
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		CustomRules:     "",
		Overwrite:       false,
		AiRobotsTxt:     false,
		LastModified:    false,
		AiRobotsTxtURL:  "https://raw.githubusercontent.com/ai-robots-txt/ai.robots.txt/refs/heads/main/robots.txt",
		CacheTTL:        300, // 5 minutes default
		MaxRetries:      3,
		RequestTimeout:  10,
		FallbackContent: "",
		EnableMetrics:   false,
	}
}

type responseWriter struct {
	buffer       bytes.Buffer
	lastModified bool
	wroteHeader  bool

	http.ResponseWriter
	backendStatusCode int
	statusCode        int
}

// RobotsTxtPlugin a robots.txt plugin with enhanced caching and configuration.
type RobotsTxtPlugin struct {
	customRules     string
	overwrite       bool
	aiRobotsTxt     bool
	lastModified    bool
	aiRobotsTxtURL  string
	aiRobotsTxtPath string
	cacheTTL        time.Duration
	maxRetries      int
	requestTimeout  time.Duration
	fallbackContent string
	enableMetrics   bool
	next            http.Handler
	
	// Cache for external content with mutex for thread safety
	cache      map[string]*cacheEntry
	cacheMutex sync.RWMutex
	
	// HTTP client with timeout for external requests
	httpClient *http.Client
	
	// Metrics counters
	cacheHits      int64
	cacheMisses    int64
	externalCalls  int64
	errors         int64
	metricsMutex   sync.RWMutex
}

// New creates a new enhanced RobotsTxt plugin with caching and improved configuration.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Validate configuration
	if len(config.CustomRules) == 0 && !config.AiRobotsTxt {
		return nil, fmt.Errorf("set customRules or set aiRobotsTxt to true")
	}
	
	// Set defaults for optional configuration
	if config.CacheTTL <= 0 {
		config.CacheTTL = 300 // 5 minutes
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 10
	}
	if config.AiRobotsTxtURL == "" {
		config.AiRobotsTxtURL = "https://raw.githubusercontent.com/ai-robots-txt/ai.robots.txt/refs/heads/main/robots.txt"
	}
	
	// Validate file path if specified
	if config.AiRobotsTxtPath != "" {
		if !filepath.IsAbs(config.AiRobotsTxtPath) {
			return nil, fmt.Errorf("aiRobotsTxtPath must be an absolute path: %s", config.AiRobotsTxtPath)
		}
		if _, err := os.Stat(config.AiRobotsTxtPath); os.IsNotExist(err) {
			log.Printf("Warning: aiRobotsTxtPath does not exist: %s", config.AiRobotsTxtPath)
		}
	}

	plugin := &RobotsTxtPlugin{
		customRules:     config.CustomRules,
		overwrite:       config.Overwrite,
		aiRobotsTxt:     config.AiRobotsTxt,
		lastModified:    config.LastModified,
		aiRobotsTxtURL:  config.AiRobotsTxtURL,
		aiRobotsTxtPath: config.AiRobotsTxtPath,
		cacheTTL:        time.Duration(config.CacheTTL) * time.Second,
		maxRetries:      config.MaxRetries,
		requestTimeout:  time.Duration(config.RequestTimeout) * time.Second,
		fallbackContent: config.FallbackContent,
		enableMetrics:   config.EnableMetrics,
		next:            next,
		cache:           make(map[string]*cacheEntry),
		httpClient: &http.Client{
			Timeout: time.Duration(config.RequestTimeout) * time.Second,
		},
	}
	
	if config.EnableMetrics {
		log.Printf("RobotsTxt plugin initialized with metrics enabled (cache TTL: %v)", plugin.cacheTTL)
	}

	return plugin, nil
}

func (p *RobotsTxtPlugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if strings.ToLower(req.URL.Path) != "/robots.txt" {
		p.next.ServeHTTP(rw, req)
		return
	}

	startTime := time.Now()
	
	wrappedWriter := &responseWriter{
		lastModified:      p.lastModified,
		ResponseWriter:    rw,
		backendStatusCode: http.StatusOK,
		statusCode:        http.StatusOK,
	}
	p.next.ServeHTTP(wrappedWriter, req)

	if wrappedWriter.backendStatusCode == http.StatusNotModified {
		return
	}

	var body string

	// Include original content unless overwrite is enabled or backend returned 404
	if !p.overwrite && wrappedWriter.backendStatusCode != http.StatusNotFound {
		body = wrappedWriter.buffer.String() + "\n"
	}

	// Add plugin attribution
	body += "# The following content was added on the fly by the Pangolin Robots.txt Traefik plugin: " +
		"https://github.com/hhftechnology/traefik-pangolin-robots-txt\n"

	// Fetch and append AI robots.txt content if enabled
	if p.aiRobotsTxt {
		aiRobotsTxt, err := p.fetchAiRobotsTxtWithCache()
		if err != nil {
			p.incrementErrorCount()
			log.Printf("unable to fetch ai.robots.txt: %v", err)
			
			// Use fallback content if available
			if p.fallbackContent != "" {
				log.Printf("using fallback content for ai.robots.txt")
				aiRobotsTxt = p.fallbackContent
			}
		}
		body += aiRobotsTxt
	}
	
	// Append custom rules
	body += p.customRules

	// Write the final response
	_, err := rw.Write([]byte(body))
	if err != nil {
		p.incrementErrorCount()
		log.Printf("unable to write body: %v", err)
	}
	
	// Log metrics if enabled
	if p.enableMetrics {
		duration := time.Since(startTime)
		log.Printf("RobotsTxt request completed in %v (backend status: %d)", 
			duration, wrappedWriter.backendStatusCode)
	}
}

// fetchAiRobotsTxtWithCache fetches AI robots.txt content with intelligent caching
func (p *RobotsTxtPlugin) fetchAiRobotsTxtWithCache() (string, error) {
	source := p.aiRobotsTxtPath
	if source == "" {
		source = p.aiRobotsTxtURL
	}
	
	// Check cache first
	p.cacheMutex.RLock()
	if entry, exists := p.cache[source]; exists && time.Now().Before(entry.expiresAt) {
		p.cacheMutex.RUnlock()
		p.incrementCacheHit()
		if p.enableMetrics {
			log.Printf("Cache hit for source: %s", source)
		}
		return entry.content, nil
	}
	p.cacheMutex.RUnlock()
	
	p.incrementCacheMiss()
	if p.enableMetrics {
		log.Printf("Cache miss for source: %s", source)
	}
	
	// Fetch fresh content
	var content string
	var err error
	
	if p.aiRobotsTxtPath != "" {
		content, err = p.fetchFromFile(p.aiRobotsTxtPath)
	} else {
		content, err = p.fetchFromURL(p.aiRobotsTxtURL)
	}
	
	if err != nil {
		return "", fmt.Errorf("failed to fetch AI robots.txt from %s: %w", source, err)
	}
	
	// Validate content
	if err := p.validateRobotsContent(content); err != nil {
		log.Printf("Warning: fetched robots.txt content validation failed: %v", err)
	}
	
	// Cache the content
	p.cacheMutex.Lock()
	p.cache[source] = &cacheEntry{
		content:   content,
		expiresAt: time.Now().Add(p.cacheTTL),
		source:    source,
	}
	p.cacheMutex.Unlock()
	
	if p.enableMetrics {
		log.Printf("Cached content from %s (TTL: %v)", source, p.cacheTTL)
	}
	
	return content, nil
}

// fetchFromFile reads robots.txt content from a local file
func (p *RobotsTxtPlugin) fetchFromFile(filePath string) (string, error) {
	if p.enableMetrics {
		log.Printf("Reading robots.txt from file: %s", filePath)
	}
	
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	
	return string(content), nil
}

// fetchFromURL fetches robots.txt content from a URL with retry logic
func (p *RobotsTxtPlugin) fetchFromURL(url string) (string, error) {
	p.incrementExternalCall()
	
	var lastErr error
	for attempt := 1; attempt <= p.maxRetries; attempt++ {
		if p.enableMetrics && attempt > 1 {
			log.Printf("Retry attempt %d/%d for URL: %s", attempt, p.maxRetries, url)
		}
		
		resp, err := p.httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed (attempt %d): %w", attempt, err)
			if attempt < p.maxRetries {
				time.Sleep(time.Duration(attempt) * time.Second) 
				continue
			}
			break
		}

		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing HTTP response: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP status code %d (attempt %d)", resp.StatusCode, attempt)
			if attempt < p.maxRetries && resp.StatusCode >= 500 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			break
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body (attempt %d): %w", attempt, err)
			if attempt < p.maxRetries {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			break
		}
		
		if p.enableMetrics {
			log.Printf("Successfully fetched %d bytes from %s", len(content), url)
		}
		
		return string(content), nil
	}
	
	return "", lastErr
}

// validateRobotsContent performs basic validation on robots.txt content
func (p *RobotsTxtPlugin) validateRobotsContent(content string) error {
	if len(content) == 0 {
		return fmt.Errorf("content is empty")
	}
	
	// Basic validation: check for common robots.txt patterns
	lines := strings.Split(content, "\n")
	hasUserAgent := false
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "user-agent:") {
			hasUserAgent = true
			break
		}
	}
	
	if !hasUserAgent {
		return fmt.Errorf("no User-agent directive found")
	}
	
	return nil
}

// Metrics methods for monitoring
func (p *RobotsTxtPlugin) incrementCacheHit() {
	if p.enableMetrics {
		p.metricsMutex.Lock()
		p.cacheHits++
		p.metricsMutex.Unlock()
	}
}

func (p *RobotsTxtPlugin) incrementCacheMiss() {
	if p.enableMetrics {
		p.metricsMutex.Lock()
		p.cacheMisses++
		p.metricsMutex.Unlock()
	}
}

func (p *RobotsTxtPlugin) incrementExternalCall() {
	if p.enableMetrics {
		p.metricsMutex.Lock()
		p.externalCalls++
		p.metricsMutex.Unlock()
	}
}

func (p *RobotsTxtPlugin) incrementErrorCount() {
	if p.enableMetrics {
		p.metricsMutex.Lock()
		p.errors++
		p.metricsMutex.Unlock()
	}
}

// GetMetrics returns current plugin metrics (useful for monitoring)
func (p *RobotsTxtPlugin) GetMetrics() map[string]int64 {
	if !p.enableMetrics {
		return nil
	}
	
	p.metricsMutex.RLock()
	defer p.metricsMutex.RUnlock()
	
	return map[string]int64{
		"cache_hits":     p.cacheHits,
		"cache_misses":   p.cacheMisses,
		"external_calls": p.externalCalls,
		"errors":         p.errors,
	}
}

// Standard response writer methods (unchanged from original)
func (r *responseWriter) WriteHeader(statusCode int) {
	if !r.lastModified {
		r.ResponseWriter.Header().Del("Last-Modified")
	}

	r.wroteHeader = true
	r.backendStatusCode = statusCode
	if statusCode != http.StatusNotFound {
		r.statusCode = statusCode
	} else {
		r.statusCode = http.StatusOK
	}

	r.ResponseWriter.Header().Set("Content-Type", "text/plain")
	r.ResponseWriter.Header().Del("Content-Length")
	r.ResponseWriter.WriteHeader(r.statusCode)
}

func (r *responseWriter) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.buffer.Write(p)
}

func (r *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("%T is not a http.Hijacker", r.ResponseWriter)
	}
	return hijacker.Hijack()
}

func (r *responseWriter) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}