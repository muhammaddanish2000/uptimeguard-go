package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const dataFile = "uptimeguard-data.json"
const reportFile = "uptime-report.md"

type Site struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	ExpectedStatus int    `json:"expected_status"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	CreatedAt      string `json:"created_at"`
}

type CheckResult struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Online     bool   `json:"online"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error"`
	CheckedAt  string `json:"checked_at"`
}

type Store struct {
	Sites   []Site        `json:"sites"`
	History []CheckResult `json:"history"`
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	command := strings.ToLower(os.Args[1])

	switch command {
	case "init":
		handleInit()
	case "add":
		handleAdd(os.Args[2:])
	case "list":
		handleList()
	case "remove":
		handleRemove(os.Args[2:])
	case "check":
		handleCheck(os.Args[2:])
	case "stats":
		handleStats()
	case "report":
		handleReport(os.Args[2:])
	case "help":
		printHelp()
	default:
		fmt.Println("Unknown command:", command)
		fmt.Println()
		printHelp()
	}
}

func printHelp() {
	fmt.Println("UptimeGuard Go")
	fmt.Println("A simple Go CLI tool for monitoring website uptime.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run main.go init")
	fmt.Println("  go run main.go add --name Portfolio --url https://example.com")
	fmt.Println("  go run main.go list")
	fmt.Println("  go run main.go check")
	fmt.Println("  go run main.go check --name Portfolio")
	fmt.Println("  go run main.go stats")
	fmt.Println("  go run main.go report")
	fmt.Println("  go run main.go remove --name Portfolio")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init      Create sample data file")
	fmt.Println("  add       Add a website to monitor")
	fmt.Println("  list      List monitored websites")
	fmt.Println("  check     Run uptime checks")
	fmt.Println("  stats     Show uptime statistics")
	fmt.Println("  report    Generate Markdown report")
	fmt.Println("  remove    Remove a website")
	fmt.Println("  help      Show help menu")
}

func handleInit() {
	if fileExists(dataFile) {
		fmt.Println("Data file already exists:", dataFile)
		return
	}

	now := time.Now().Format(time.RFC3339)

	store := Store{
		Sites: []Site{
			{
				Name:           "GitHub",
				URL:            "https://github.com",
				ExpectedStatus: 200,
				TimeoutSeconds: 10,
				CreatedAt:      now,
			},
			{
				Name:           "Example Website",
				URL:            "https://example.com",
				ExpectedStatus: 200,
				TimeoutSeconds: 10,
				CreatedAt:      now,
			},
		},
		History: []CheckResult{},
	}

	err := saveStore(store)

	if err != nil {
		exitWithError(err)
	}

	fmt.Println("Project initialized successfully.")
	fmt.Println("Created:", dataFile)
	fmt.Println()
	fmt.Println("Try running:")
	fmt.Println("  go run main.go list")
	fmt.Println("  go run main.go check")
}

func handleAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)

	name := fs.String("name", "", "Website name")
	siteURL := fs.String("url", "", "Website URL")
	expectedStatus := fs.Int("status", 200, "Expected HTTP status code")
	timeout := fs.Int("timeout", 10, "Request timeout in seconds")

	_ = fs.Parse(args)

	cleanName := strings.TrimSpace(*name)
	cleanURL := strings.TrimSpace(*siteURL)

	if cleanName == "" {
		exitWithError(errors.New("website name is required"))
	}

	if cleanURL == "" {
		exitWithError(errors.New("website URL is required"))
	}

	if !isValidURL(cleanURL) {
		exitWithError(errors.New("invalid URL. Please include http:// or https://"))
	}

	if *expectedStatus < 100 || *expectedStatus > 599 {
		exitWithError(errors.New("expected status must be between 100 and 599"))
	}

	if *timeout < 1 || *timeout > 120 {
		exitWithError(errors.New("timeout must be between 1 and 120 seconds"))
	}

	store := loadStore()

	for _, site := range store.Sites {
		if strings.EqualFold(site.Name, cleanName) {
			exitWithError(errors.New("site name already exists"))
		}
	}

	store.Sites = append(store.Sites, Site{
		Name:           cleanName,
		URL:            cleanURL,
		ExpectedStatus: *expectedStatus,
		TimeoutSeconds: *timeout,
		CreatedAt:      time.Now().Format(time.RFC3339),
	})

	err := saveStore(store)

	if err != nil {
		exitWithError(err)
	}

	fmt.Println("Website added successfully.")
	fmt.Println("Name:", cleanName)
	fmt.Println("URL:", cleanURL)
}

func handleList() {
	store := loadStore()

	if len(store.Sites) == 0 {
		fmt.Println("No websites added yet.")
		fmt.Println("Add one using:")
		fmt.Println("  go run main.go add --name Portfolio --url https://example.com")
		return
	}

	fmt.Println("Monitored Websites")
	fmt.Println("------------------")

	for index, site := range store.Sites {
		fmt.Printf("%d. %s\n", index+1, site.Name)
		fmt.Printf("   URL: %s\n", site.URL)
		fmt.Printf("   Expected Status: %d\n", site.ExpectedStatus)
		fmt.Printf("   Timeout: %d seconds\n", site.TimeoutSeconds)
		fmt.Printf("   Created: %s\n", site.CreatedAt)
		fmt.Println()
	}
}

func handleRemove(args []string) {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	name := fs.String("name", "", "Website name to remove")

	_ = fs.Parse(args)

	cleanName := strings.TrimSpace(*name)

	if cleanName == "" {
		exitWithError(errors.New("website name is required"))
	}

	store := loadStore()
	updatedSites := make([]Site, 0)
	removed := false

	for _, site := range store.Sites {
		if strings.EqualFold(site.Name, cleanName) {
			removed = true
			continue
		}

		updatedSites = append(updatedSites, site)
	}

	if !removed {
		exitWithError(errors.New("website not found"))
	}

	store.Sites = updatedSites

	err := saveStore(store)

	if err != nil {
		exitWithError(err)
	}

	fmt.Println("Website removed successfully:", cleanName)
}

func handleCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	name := fs.String("name", "", "Optional website name to check")

	_ = fs.Parse(args)

	store := loadStore()

	if len(store.Sites) == 0 {
		fmt.Println("No websites available to check.")
		fmt.Println("Add one using:")
		fmt.Println("  go run main.go add --name Portfolio --url https://example.com")
		return
	}

	selectedSites := store.Sites

	if strings.TrimSpace(*name) != "" {
		selectedSites = filterSitesByName(store.Sites, *name)

		if len(selectedSites) == 0 {
			exitWithError(errors.New("website not found"))
		}
	}

	fmt.Println("Running uptime checks...")
	fmt.Println()

	for _, site := range selectedSites {
		result := checkSite(site)
		store.History = append(store.History, result)

		status := "DOWN"

		if result.Online {
			status = "UP"
		}

		fmt.Printf("[%s] %s\n", status, result.Name)
		fmt.Printf("URL: %s\n", result.URL)
		fmt.Printf("Status Code: %d\n", result.StatusCode)
		fmt.Printf("Duration: %dms\n", result.DurationMs)

		if result.Error != "" {
			fmt.Printf("Error: %s\n", result.Error)
		}

		fmt.Println()
	}

	store.History = limitHistory(store.History, 1000)

	err := saveStore(store)

	if err != nil {
		exitWithError(err)
	}

	fmt.Println("Check completed. Results saved to", dataFile)
}

func handleStats() {
	store := loadStore()

	if len(store.Sites) == 0 {
		fmt.Println("No websites added yet.")
		return
	}

	if len(store.History) == 0 {
		fmt.Println("No check history yet.")
		fmt.Println("Run:")
		fmt.Println("  go run main.go check")
		return
	}

	stats := buildStats(store)

	fmt.Println("Uptime Statistics")
	fmt.Println("-----------------")

	for _, item := range stats {
		fmt.Printf("%s\n", item.Name)
		fmt.Printf("  Checks: %d\n", item.TotalChecks)
		fmt.Printf("  Online: %d\n", item.SuccessChecks)
		fmt.Printf("  Uptime: %.2f%%\n", item.UptimePercent)
		fmt.Printf("  Average Response: %dms\n", item.AverageDurationMs)

		if item.LastCheckedAt != "" {
			fmt.Printf("  Last Checked: %s\n", item.LastCheckedAt)
			fmt.Printf("  Last Status: %s\n", item.LastStatus)
		}

		fmt.Println()
	}
}

func handleReport(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	output := fs.String("output", reportFile, "Report output filename")

	_ = fs.Parse(args)

	store := loadStore()
	report := buildMarkdownReport(store)

	err := os.WriteFile(*output, []byte(report), 0644)

	if err != nil {
		exitWithError(err)
	}

	fmt.Println("Report generated successfully:", *output)
}

func checkSite(site Site) CheckResult {
	start := time.Now()

	timeout := time.Duration(site.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, site.URL, nil)

	if err != nil {
		return CheckResult{
			Name:       site.Name,
			URL:        site.URL,
			StatusCode: 0,
			Online:     false,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
			CheckedAt:  time.Now().Format(time.RFC3339),
		}
	}

	request.Header.Set("User-Agent", "UptimeGuard-Go/1.0")

	client := &http.Client{}

	response, err := client.Do(request)

	if err != nil {
		return CheckResult{
			Name:       site.Name,
			URL:        site.URL,
			StatusCode: 0,
			Online:     false,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
			CheckedAt:  time.Now().Format(time.RFC3339),
		}
	}

	defer response.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))

	duration := time.Since(start).Milliseconds()

	return CheckResult{
		Name:       site.Name,
		URL:        site.URL,
		StatusCode: response.StatusCode,
		Online:     response.StatusCode == site.ExpectedStatus,
		DurationMs: duration,
		Error:      "",
		CheckedAt:  time.Now().Format(time.RFC3339),
	}
}

type SiteStats struct {
	Name              string
	TotalChecks       int
	SuccessChecks     int
	UptimePercent     float64
	AverageDurationMs int64
	LastCheckedAt      string
	LastStatus         string
}

func buildStats(store Store) []SiteStats {
	resultsBySite := make(map[string][]CheckResult)

	for _, result := range store.History {
		resultsBySite[result.Name] = append(resultsBySite[result.Name], result)
	}

	stats := make([]SiteStats, 0)

	for _, site := range store.Sites {
		results := resultsBySite[site.Name]

		total := len(results)
		success := 0
		var totalDuration int64 = 0

		for _, result := range results {
			if result.Online {
				success++
			}

			totalDuration += result.DurationMs
		}

		uptime := 0.0
		var averageDuration int64 = 0

		if total > 0 {
			uptime = (float64(success) / float64(total)) * 100
			averageDuration = totalDuration / int64(total)
		}

		lastStatus := "No checks yet"
		lastChecked := ""

		if total > 0 {
			last := results[total-1]
			lastChecked = last.CheckedAt

			if last.Online {
				lastStatus = "UP"
			} else {
				lastStatus = "DOWN"
			}
		}

		stats = append(stats, SiteStats{
			Name:              site.Name,
			TotalChecks:       total,
			SuccessChecks:     success,
			UptimePercent:     uptime,
			AverageDurationMs: averageDuration,
			LastCheckedAt:      lastChecked,
			LastStatus:         lastStatus,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Name < stats[j].Name
	})

	return stats
}

func buildMarkdownReport(store Store) string {
	stats := buildStats(store)

	siteRows := ""

	for _, site := range store.Sites {
		siteRows += fmt.Sprintf(
			"| %s | %s | %d | %ds |\n",
			site.Name,
			site.URL,
			site.ExpectedStatus,
			site.TimeoutSeconds,
		)
	}

	statRows := ""

	for _, item := range stats {
		statRows += fmt.Sprintf(
			"| %s | %d | %d | %.2f%% | %dms | %s |\n",
			item.Name,
			item.TotalChecks,
			item.SuccessChecks,
			item.UptimePercent,
			item.AverageDurationMs,
			item.LastStatus,
		)
	}

	recentRows := ""

	recent := getRecentHistory(store.History, 10)

	for _, result := range recent {
		status := "DOWN"

		if result.Online {
			status = "UP"
		}

		errorText := result.Error

		if errorText == "" {
			errorText = "-"
		}

		recentRows += fmt.Sprintf(
			"| %s | %s | %d | %s | %dms | %s |\n",
			result.CheckedAt,
			result.Name,
			result.StatusCode,
			status,
			result.DurationMs,
			errorText,
		)
	}

	if siteRows == "" {
		siteRows = "| No sites added | - | - | - |\n"
	}

	if statRows == "" {
		statRows = "| No statistics available | - | - | - | - | - |\n"
	}

	if recentRows == "" {
		recentRows = "| No checks yet | - | - | - | - | - |\n"
	}

	return fmt.Sprintf(`# UptimeGuard Website Monitoring Report

Generated at: %s

## Summary

UptimeGuard Go is a command-line website monitoring tool built with Go. It checks website availability, records response history, calculates uptime statistics, and generates Markdown reports.

## Monitored Websites

| Name | URL | Expected Status | Timeout |
|---|---|---:|---:|
%s
## Uptime Statistics

| Website | Total Checks | Successful Checks | Uptime | Average Response | Last Status |
|---|---:|---:|---:|---:|---|
%s
## Recent Checks

| Checked At | Website | Status Code | Result | Duration | Error |
|---|---|---:|---|---:|---|
%s
## Recommended Next Actions

1. Review websites with low uptime percentage.
2. Check websites that return unexpected status codes.
3. Increase monitoring frequency for important production websites.
4. Add important landing pages, client websites, and portfolio projects.
5. Keep generated reports inside the repository for progress tracking.

## Suggested Commit Messages

- feat(cli): add website uptime checker
- feat(report): add markdown uptime report generator
- feat(storage): add local json history storage
- docs(readme): add project setup and usage guide
- refactor(cli): improve command handling

`, time.Now().Format(time.RFC3339), siteRows, statRows, recentRows)
}

func getRecentHistory(history []CheckResult, limit int) []CheckResult {
	if len(history) == 0 {
		return []CheckResult{}
	}

	copied := make([]CheckResult, len(history))
	copy(copied, history)

	sort.Slice(copied, func(i, j int) bool {
		return copied[i].CheckedAt > copied[j].CheckedAt
	})

	if len(copied) > limit {
		return copied[:limit]
	}

	return copied
}

func filterSitesByName(sites []Site, name string) []Site {
	cleanName := strings.TrimSpace(name)
	filtered := make([]Site, 0)

	for _, site := range sites {
		if strings.EqualFold(site.Name, cleanName) {
			filtered = append(filtered, site)
		}
	}

	return filtered
}

func limitHistory(history []CheckResult, limit int) []CheckResult {
	if len(history) <= limit {
		return history
	}

	return history[len(history)-limit:]
}

func loadStore() Store {
	if !fileExists(dataFile) {
		return Store{
			Sites:   []Site{},
			History: []CheckResult{},
		}
	}

	content, err := os.ReadFile(dataFile)

	if err != nil {
		exitWithError(err)
	}

	if len(content) == 0 {
		return Store{
			Sites:   []Site{},
			History: []CheckResult{},
		}
	}

	var store Store

	err = json.Unmarshal(content, &store)

	if err != nil {
		exitWithError(err)
	}

	if store.Sites == nil {
		store.Sites = []Site{}
	}

	if store.History == nil {
		store.History = []CheckResult{}
	}

	return store
}

func saveStore(store Store) error {
	content, err := json.MarshalIndent(store, "", "  ")

	if err != nil {
		return err
	}

	return os.WriteFile(dataFile, content, 0644)
}

func isValidURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)

	if err != nil {
		return false
	}

	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func exitWithError(err error) {
	fmt.Println("Error:", err.Error())
	os.Exit(1)
}
