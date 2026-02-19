package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ScriptRange defines a script to detect
type ScriptRange struct {
	Name  string
	Regex *regexp.Regexp
}

// Available scripts to choose from (order matters for display)
var scriptOptions = []ScriptRange{
	{"Han (Chinese/Japanese Kanji)", regexp.MustCompile(`[\p{Han}]`)},
	{"Devanagari (Hindi/Sanskrit)", regexp.MustCompile(`[\p{Devanagari}]`)},
	{"Cyrillic (Russian/etc)", regexp.MustCompile(`[\p{Cyrillic}]`)},
	{"Hiragana (Japanese)", regexp.MustCompile(`[\p{Hiragana}]`)},
	{"Katakana (Japanese)", regexp.MustCompile(`[\p{Katakana}]`)},
	{"Hangul (Korean)", regexp.MustCompile(`[\p{Hangul}]`)},
	{"Thai", regexp.MustCompile(`[\p{Thai}]`)},
	{"Arabic", regexp.MustCompile(`[\p{Arabic}]`)},
	{"Tamil", regexp.MustCompile(`[\p{Tamil}]`)},
	{"Bengali", regexp.MustCompile(`[\p{Bengali}]`)},
	{"Greek", regexp.MustCompile(`[\p{Greek}]`)},
	{"Hebrew", regexp.MustCompile(`[\p{Hebrew}]`)},
}

var latinRe = regexp.MustCompile(`[a-zA-Z]`)

// Match represents a single mixed-script detection with context
type Match struct {
	TriggerPhrase string `json:"trigger"`        // The exact phrase to search for
	ContextBefore string `json:"context_before"` // A few words before for context
	ContextAfter  string `json:"context_after"`  // A few words after for context
	FullSegment   string `json:"full_segment"`   // The full segment for reference
}

// CrawlStatus represents the status of a crawl attempt
type CrawlStatus int

const (
	StatusSuccess CrawlStatus = iota
	StatusError
)

// CrawlResult represents a crawl result (success or error)
type CrawlResult struct {
	URL     string
	Status  CrawlStatus
	Error   error
	Snippet string
}

// Progress update for live display
type Progress struct {
	URL        string
	Status     string
	IsError    bool
	Findings   int
	StatusCode int
}

// Styles for output
var (
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleFound   = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
)

// Extract visible text from HTML document
func extractVisibleText(doc *goquery.Document) string {
	var sb strings.Builder

	doc.Find("body").Each(func(i int, s *goquery.Selection) {
		s.Find("script, style, noscript, iframe, svg, [hidden], head, meta, link").Remove()
		s.Find("*").Each(func(_ int, el *goquery.Selection) {
			if el.Children().Length() == 0 {
				text := strings.TrimSpace(el.Text())
				if text != "" {
					sb.WriteString(text + " ")
				}
			}
		})
	})

	return sb.String()
}

// Find mixed-script instances in text
func findMixedInstances(text string, targetScript *regexp.Regexp) []Match {
	var findings []Match
	seen := make(map[string]bool)

	// Split into segments by sentence boundaries
	segments := regexp.MustCompile(`[.!?\n]+`).Split(text, -1)

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if len(segment) < 3 {
			continue
		}

		hasLatin := latinRe.MatchString(segment)
		hasTarget := targetScript.MatchString(segment)

		if hasLatin && hasTarget {
			// Find the exact trigger phrases within this segment
			triggerMatches := extractTriggerPhrases(segment, targetScript)

			for _, m := range triggerMatches {
				// Deduplicate by trigger phrase
				if !seen[m.TriggerPhrase] {
					seen[m.TriggerPhrase] = true
					m.FullSegment = cleanSegment(segment)
					findings = append(findings, m)
				}
			}
		}
	}

	return findings
}

// extractTriggerPhrases finds the specific phrases containing target script chars
func extractTriggerPhrases(segment string, targetScript *regexp.Regexp) []Match {
	var matches []Match

	// Split by spaces to get words
	words := strings.Fields(segment)

	// Find "phrase windows" that contain target script
	// A phrase is: a few words before + the word(s) with target chars + a few words after
	for i, word := range words {
		if targetScript.MatchString(word) {
			// Found a word with target script!
			// Build a trigger phrase: expand to include adjacent non-target words for context
			triggerPhrase := word
			contextBefore := ""
			contextAfter := ""

			// Grab 1-2 words before for context (but not the trigger itself)
			if i > 0 {
				contextBefore = words[i-1]
				if i > 1 && !targetScript.MatchString(words[i-1]) {
					contextBefore = words[i-2] + " " + words[i-1]
				}
			}

			// Check if next word also has target script (merge them)
			endIdx := i
			for j := i + 1; j < len(words) && j < i+3; j++ {
				if targetScript.MatchString(words[j]) || (j == i+1 && containsLatinOrTarget(words[j], targetScript)) {
					triggerPhrase += " " + words[j]
					endIdx = j
				} else {
					break
				}
			}

			// Grab 1-2 words after for context
			if endIdx+1 < len(words) {
				contextAfter = words[endIdx+1]
				if endIdx+2 < len(words) && !targetScript.MatchString(words[endIdx+2]) {
					contextAfter += " " + words[endIdx+2]
				}
			}

			matches = append(matches, Match{
				TriggerPhrase: triggerPhrase,
				ContextBefore: contextBefore,
				ContextAfter:  contextAfter,
			})
		}
	}

	return matches
}

// containsLatinOrTarget checks if a word has Latin or target script chars
func containsLatinOrTarget(word string, targetScript *regexp.Regexp) bool {
	return latinRe.MatchString(word) || targetScript.MatchString(word)
}

// Clean a segment for output
func cleanSegment(s string) string {
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:297] + "..."
	}
	return s
}

// Shorten URL for display
func shortURL(rawURL string, maxLen int) string {
	if len(rawURL) <= maxLen {
		return rawURL
	}
	return rawURL[:maxLen-3] + "..."
}

// Crawl a page and collect findings
func crawlPage(rawURL string, visited *sync.Map, results chan<- CrawlResult, progress chan<- Progress, wg *sync.WaitGroup, client *http.Client, printMu *sync.Mutex) {
	defer wg.Done()

	u, err := url.Parse(rawURL)
	if err != nil {
		results <- CrawlResult{URL: rawURL, Status: StatusError, Error: fmt.Errorf("invalid URL: %v", err)}
		return
	}
	domain := u.Host

	visited.Store(rawURL, true)

	// Report progress: starting
	progress <- Progress{URL: rawURL, Status: "fetching...", IsError: false}

	resp, err := client.Get(rawURL)
	if err != nil {
		errMsg := err.Error()
		// Detect common error types
		if strings.Contains(errMsg, "timeout") {
			errMsg = "TIMEOUT (request took too long)"
		} else if strings.Contains(errMsg, "connection refused") {
			errMsg = "CONNECTION REFUSED (server not reachable)"
		} else if strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "lookup") {
			errMsg = "DNS FAILED (could not resolve host - network blocked or invalid domain)"
		} else if strings.Contains(errMsg, "connection reset") {
			errMsg = "CONNECTION RESET (network interrupted)"
		} else if strings.Contains(errMsg, "TLS") || strings.Contains(errMsg, "certificate") {
			errMsg = "TLS/CERT ERROR (HTTPS issue)"
		}

		results <- CrawlResult{URL: rawURL, Status: StatusError, Error: fmt.Errorf("%s", errMsg)}
		progress <- Progress{URL: rawURL, Status: errMsg, IsError: true}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		statusMsg := fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status)
		results <- CrawlResult{URL: rawURL, Status: StatusError, Error: fmt.Errorf("%s", statusMsg)}
		progress <- Progress{URL: rawURL, Status: statusMsg, IsError: true, StatusCode: resp.StatusCode}
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		results <- CrawlResult{URL: rawURL, Status: StatusError, Error: fmt.Errorf("failed to parse HTML: %v", err)}
		progress <- Progress{URL: rawURL, Status: "HTML parse error", IsError: true}
		return
	}

	fullText := extractVisibleText(doc)

	if len(strings.TrimSpace(fullText)) > 0 {
		results <- CrawlResult{URL: rawURL, Status: StatusSuccess, Snippet: fullText}
	}

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		link, err := url.Parse(href)
		if err != nil {
			return
		}

		fullLink := u.ResolveReference(link).String()
		parsed, _ := url.Parse(fullLink)

		if parsed.Host == domain && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			if _, seen := visited.LoadOrStore(fullLink, true); !seen {
				wg.Add(1)
				go crawlPage(fullLink, visited, results, progress, wg, client, printMu)
			}
		}
	})
}

// Extract first N chars from domain for filename
func getDomainPrefix(rawURL string, n int) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	host := u.Host
	host = strings.TrimPrefix(host, "www.")
	var result []rune
	for _, r := range host {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result = append(result, r)
			if len(result) >= n {
				break
			}
		}
	}
	if len(result) == 0 {
		return "unknown"
	}
	return string(result)
}

// Write results to file
func writeResults(filename string, rootURL string, scriptName string, allFindings map[string][]Match, errors []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	fmt.Fprintf(writer, "=== Mixed Script Detector Results ===\n")
	fmt.Fprintf(writer, "Date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(writer, "Root URL: %s\n", rootURL)
	fmt.Fprintf(writer, "Target Script: %s\n", scriptName)
	fmt.Fprintf(writer, "=====================================\n\n")

	if len(errors) > 0 {
		fmt.Fprintf(writer, "=== Errors Encountered ===\n")
		for _, e := range errors {
			fmt.Fprintf(writer, "  - %s\n", e)
		}
		fmt.Fprintf(writer, "\n")
	}

	if len(allFindings) == 0 {
		fmt.Fprintf(writer, "No mixed-script instances found.\n")
		return nil
	}

	for urlStr, matches := range allFindings {
		if len(matches) == 0 {
			continue
		}

		fmt.Fprintf(writer, "--- URL: %s ---\n", urlStr)
		fmt.Fprintf(writer, "Found %d instance(s):\n\n", len(matches))

		for i, m := range matches {
			fmt.Fprintf(writer, "[%d] ", i+1)

			// Format: "context before [TRIGGER PHRASE] context after"
			// The trigger phrase in brackets is what to Ctrl+F for
			if m.ContextBefore != "" {
				fmt.Fprintf(writer, "%s ", m.ContextBefore)
			}
			fmt.Fprintf(writer, "[%s]", m.TriggerPhrase)
			if m.ContextAfter != "" {
				fmt.Fprintf(writer, " %s", m.ContextAfter)
			}
			fmt.Fprintf(writer, "\n\n")
		}
	}

	return nil
}

// JSONOutput represents the JSON export structure
type JSONOutput struct {
	Metadata struct {
		Date         string `json:"date"`
		RootURL      string `json:"root_url"`
		TargetScript string `json:"target_script"`
		TotalPages   int    `json:"total_pages_with_findings"`
		TotalMatches int    `json:"total_matches"`
	} `json:"metadata"`
	Errors  []string `json:"errors,omitempty"`
	Results []struct {
		URL     string  `json:"url"`
		Count   int     `json:"match_count"`
		Matches []Match `json:"matches"`
	} `json:"results"`
}

// writeJSON exports results to JSON format
func writeJSON(filename string, rootURL string, scriptName string, allFindings map[string][]Match, errors []string) error {
	output := JSONOutput{}
	output.Metadata.Date = time.Now().Format("2006-01-02 15:04:05")
	output.Metadata.RootURL = rootURL
	output.Metadata.TargetScript = scriptName
	output.Metadata.TotalPages = len(allFindings)
	output.Errors = errors

	totalMatches := 0
	for urlStr, matches := range allFindings {
		if len(matches) == 0 {
			continue
		}
		totalMatches += len(matches)

		result := struct {
			URL     string  `json:"url"`
			Count   int     `json:"match_count"`
			Matches []Match `json:"matches"`
		}{
			URL:     urlStr,
			Count:   len(matches),
			Matches: matches,
		}
		output.Results = append(output.Results, result)
	}
	output.Metadata.TotalMatches = totalMatches

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func runInteractive() (string, string, int, error) {
	var selectedScriptIdx string
	var startURL string
	var maxPagesStr string

	var scriptChoices []huh.Option[string]
	for i, s := range scriptOptions {
		scriptChoices = append(scriptChoices, huh.NewOption(s.Name, fmt.Sprintf("%d", i)))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select target script to detect").
				Options(scriptChoices...).
				Value(&selectedScriptIdx).
				Height(8),

			huh.NewInput().
				Title("Enter website URL").
				Placeholder("https://example.com").
				Value(&startURL),

			huh.NewInput().
				Title("Max pages to crawl (press Enter for default: 50)").
				Placeholder("50").
				Value(&maxPagesStr),
		),
	)

	err := form.Run()
	if err != nil {
		return "", "", 0, err
	}

	maxPages := 50
	if maxPagesStr != "" {
		fmt.Sscanf(maxPagesStr, "%d", &maxPages)
		if maxPages <= 0 {
			maxPages = 50
		}
	}

	var scriptName string
	var idx int
	fmt.Sscanf(selectedScriptIdx, "%d", &idx)
	if idx >= 0 && idx < len(scriptOptions) {
		scriptName = scriptOptions[idx].Name
	}

	return startURL, scriptName, maxPages, nil
}

func main() {
	var startURL string
	var scriptIdx int = 0
	var maxPages int
	var useInteractive bool = true

	args := os.Args[1:]
	for i, arg := range args {
		if strings.HasPrefix(arg, "-url=") || strings.HasPrefix(arg, "--url=") {
			startURL = strings.TrimPrefix(strings.TrimPrefix(arg, "-url="), "--url=")
			useInteractive = false
		} else if arg == "-url" || arg == "--url" {
			if i+1 < len(args) {
				startURL = args[i+1]
			}
			useInteractive = false
		} else if strings.HasPrefix(arg, "-script=") || strings.HasPrefix(arg, "--script=") {
			s := strings.TrimPrefix(strings.TrimPrefix(arg, "-script="), "--script=")
			scriptMap := map[string]int{
				"han": 0, "devanagari": 1, "cyrillic": 2, "hiragana": 3,
				"katakana": 4, "hangul": 5, "thai": 6, "arabic": 7,
				"tamil": 8, "bengali": 9, "greek": 10, "hebrew": 11,
			}
			if v, ok := scriptMap[strings.ToLower(s)]; ok {
				scriptIdx = v
			}
			useInteractive = false
		} else if strings.HasPrefix(arg, "-max=") || strings.HasPrefix(arg, "--max=") {
			fmt.Sscanf(strings.TrimPrefix(strings.TrimPrefix(arg, "-max="), "--max="), "%d", &maxPages)
			useInteractive = false
		}
	}

	if useInteractive || startURL == "" {
		var scriptName string
		var err error
		startURL, scriptName, maxPages, err = runInteractive()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for i, s := range scriptOptions {
			if s.Name == scriptName {
				scriptIdx = i
				break
			}
		}
	}

	if maxPages <= 0 {
		maxPages = 50
	}

	if startURL == "" {
		fmt.Println("Please provide a URL!")
		os.Exit(1)
	}

	if !strings.HasPrefix(startURL, "http://") && !strings.HasPrefix(startURL, "https://") {
		startURL = "https://" + startURL
	}

	targetScript := scriptOptions[scriptIdx]

	fmt.Printf("\n")
	fmt.Printf("  %s\n", styleInfo.Render("Starting mixed-script detection..."))
	fmt.Printf("  %s\n", styleDim.Render("-----------------------------------"))
	fmt.Printf("  Root URL: %s\n", startURL)
	fmt.Printf("  Target Script: %s\n", targetScript.Name)
	fmt.Printf("  Max Pages: %d\n\n", maxPages)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	var wg sync.WaitGroup
	var printMu sync.Mutex
	visited := &sync.Map{}
	resultsChan := make(chan CrawlResult, 200)
	progressChan := make(chan Progress, 200)

	wg.Add(1)
	go crawlPage(startURL, visited, resultsChan, progressChan, &wg, client, &printMu)

	allFindings := make(map[string][]Match)
	var crawlErrors []string
	done := make(chan bool)

	// Progress display goroutine
	go func() {
		pageCount := 0
		consecutiveEmpty := 0

		for {
			select {
			case prog, ok := <-progressChan:
				if !ok {
					progressChan = nil
					continue
				}
				printMu.Lock()
				if prog.IsError {
					fmt.Printf("  %s %s\n", styleError.Render("[ERROR]"), styleDim.Render(shortURL(prog.URL, 60)))
					fmt.Printf("         %s\n", styleError.Render(prog.Status))
				} else {
					fmt.Printf("  %s %s\n", styleInfo.Render("[scanning]"), styleDim.Render(shortURL(prog.URL, 60)))
				}
				printMu.Unlock()

			case result, ok := <-resultsChan:
				if !ok {
					resultsChan = nil
					continue
				}

				if result.Status == StatusError {
					errStr := fmt.Sprintf("%s: %s", result.URL, result.Error.Error())
					crawlErrors = append(crawlErrors, errStr)
					// Print error immediately
					printMu.Lock()
					fmt.Printf("  %s %s\n", styleError.Render("[ERROR]"), styleDim.Render(shortURL(result.URL, 60)))
					fmt.Printf("         %s\n", styleError.Render(result.Error.Error()))
					printMu.Unlock()
				} else if result.Status == StatusSuccess {
					instances := findMixedInstances(result.Snippet, targetScript.Regex)
					if len(instances) > 0 {
						allFindings[result.URL] = instances
						printMu.Lock()
						fmt.Printf("  %s %s\n", styleFound.Render(fmt.Sprintf("[FOUND %d]", len(instances))), result.URL)
						printMu.Unlock()
					}
				}

				pageCount++
				consecutiveEmpty = 0
				if pageCount >= maxPages {
					done <- true
					return
				}

			case <-time.After(100 * time.Millisecond):
				// Check if both channels are nil (closed and drained)
				if progressChan == nil && resultsChan == nil {
					done <- true
					return
				}
				consecutiveEmpty++
				// Safety: if nothing happens for 5 seconds, exit
				if consecutiveEmpty > 50 {
					done <- true
					return
				}
			}
		}
	}()

	// Close channels when done
	go func() {
		wg.Wait()
		close(resultsChan)
		close(progressChan)
	}()

	<-done

	domainPrefix := getDomainPrefix(startURL, 15)
	dateStr := time.Now().Format("2006-01-02")
	txtFilename := fmt.Sprintf("%s_%s_findings.txt", domainPrefix, dateStr)
	jsonFilename := fmt.Sprintf("%s_%s_findings.json", domainPrefix, dateStr)

	txtPath, _ := filepath.Abs(txtFilename)
	jsonPath, _ := filepath.Abs(jsonFilename)

	fmt.Printf("\n  %s %s\n", styleInfo.Render("Writing results to:"), txtPath)

	err := writeResults(txtFilename, startURL, targetScript.Name, allFindings, crawlErrors)
	if err != nil {
		fmt.Printf("  %s %v\n", styleError.Render("Error writing file:"), err)
		os.Exit(1)
	}

	// Also export JSON
	err = writeJSON(jsonFilename, startURL, targetScript.Name, allFindings, crawlErrors)
	if err != nil {
		fmt.Printf("  %s %v\n", styleError.Render("Error writing JSON:"), err)
	}

	totalInstances := 0
	for _, snippets := range allFindings {
		totalInstances += len(snippets)
	}

	fmt.Printf("\n")
	fmt.Printf("  %s\n", styleSuccess.Render("Done!"))
	fmt.Printf("  %s\n", styleDim.Render("-----------------------------------"))
	fmt.Printf("  Pages with findings: %d\n", len(allFindings))
	fmt.Printf("  Total mixed-script instances: %d\n", totalInstances)
	if len(crawlErrors) > 0 {
		fmt.Printf("  %s: %d\n", styleError.Render("Pages with errors"), len(crawlErrors))
	}
	fmt.Printf("  %s %s\n", styleInfo.Render("TXT:"), txtPath)
	fmt.Printf("  %s %s\n\n", styleInfo.Render("JSON:"), jsonPath)
}
