package utils

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// SearchResult represents a search result in a file
type SearchResult struct {
	LineNumber int    `json:"lineNumber"`
	Content    string `json:"content"`
}

// FormatFileSize formats a file size in bytes to a human-readable format
func FormatFileSize(size int64) (float64, string) {
	if size < 1000 {
		return float64(size), "bytes"
	} else if size < 1000000 {
		return float64(size) / 1000, "KBytes"
	} else {
		return float64(size) / 1000000, "MBytes"
	}
}

// SearchInFile searches for a term in a file with options for case sensitivity and whole word matching
func SearchInFile(filePath, searchTerm string, caseSensitive, wholeWord bool) ([]SearchResult, error) {

	// Check that the file exists before trying to open it
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the file does not exist: %s", filePath)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error to open file: %v", err)
		return nil, err
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	var results []SearchResult

	// Prepare the search term according to options
	var searchFunc func(string) bool

	if !caseSensitive {
		searchTerm = strings.ToLower(searchTerm)
	}

	if wholeWord {
		// For whole word search, use a regular expression
		var pattern string
		if caseSensitive {
			pattern = fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(searchTerm))
		} else {
			pattern = fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(searchTerm))
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}

		searchFunc = func(line string) bool {
			return re.MatchString(line)
		}
	} else {
		// For normal search
		searchFunc = func(line string) bool {
			if caseSensitive {
				return strings.Contains(line, searchTerm)
			}
			return strings.Contains(strings.ToLower(line), searchTerm)
		}
	}

	// Read the file line by line
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Check if the line contains the search term
		if searchFunc(line) {
			// Limit line length to avoid sending too much data
			if len(line) > 300 {
				line = line[:300] + "..."
			}

			results = append(results, SearchResult{
				LineNumber: lineNumber,
				Content:    line,
			})

			// Limit total number of results to avoid performance issues
			if len(results) >= 1000 {
				results = append(results, SearchResult{
					LineNumber: -1,
					Content:    "Search results limited to 1000 matches.",
				})
				break
			}
		}
	}
	// Check for reading errors
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// If no results found, return a message instead of empty array
	if len(results) == 0 {
		results = append(results, SearchResult{
			LineNumber: -1,
			Content:    "No matches found.",
		})
	}

	return results, nil
}
