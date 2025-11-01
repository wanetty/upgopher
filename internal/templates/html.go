package templates

import (
	"encoding/base64"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CreateEncodedPath creates a base64-encoded path combining current path and file name
func CreateEncodedPath(currentPath string, fileName string) string {
	decodedFilePath, _ := base64.StdEncoding.DecodeString(currentPath)
	return base64.StdEncoding.EncodeToString([]byte(filepath.Join(string(decodedFilePath), fileName)))
}

// IsTextFile determines if a file can be read as text based on its extension
func IsTextFile(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	// List of file extensions considered readable as text
	textExtensions := map[string]bool{
		".txt": true, ".md": true, ".json": true, ".xml": true, ".html": true, ".css": true,
		".js": true, ".go": true, ".py": true, ".java": true, ".c": true, ".cpp": true,
		".h": true, ".swift": true, ".rb": true, ".php": true, ".sh": true, ".bat": true,
		".log": true, ".csv": true, ".yml": true, ".yaml": true, ".toml": true, ".ini": true,
		".cfg": true, ".conf": true, ".properties": true, ".env": true, ".sql": true,
	}
	return textExtensions[ext]
}

// CreateFolderRow generates HTML for a folder row in the file listing
func CreateFolderRow(file fs.DirEntry, currentPath string, fileInfo os.FileInfo) string {
	encodedPath := CreateEncodedPath(currentPath, file.Name())
	escapedencodedFilePath := html.EscapeString(encodedPath)

	escapedFolderName := html.EscapeString(file.Name())
	folderLink := fmt.Sprintf(`<a href="/?path=%s">%s</a>`, escapedencodedFilePath, escapedFolderName)
	lastModified := fileInfo.ModTime().Format("2006-01-02 15:04:05")
	return fmt.Sprintf(`
		<tr>
			<td>%s</td>
			<td>%s</td>
			<td>-</td>
			<td>%s</td>
			<td>-</td>
			<td>
				<div class="action-buttons">
					<span>-</span>
				</div>
			</td>
		</tr>
	`, folderLink, fileInfo.Mode(), lastModified)
}

// CreateFileRow generates HTML for a file row in the file listing
func CreateFileRow(file fs.DirEntry, currentPath string, fileInfo os.FileInfo, customPaths map[string]string, formatFileSize func(int64) (float64, string)) string {
	encodedFilePath := CreateEncodedPath(currentPath, file.Name())

	escapedFileName := html.EscapeString(file.Name())
	escapedencodedFilePath := html.EscapeString(encodedFilePath)

	// Decode path
	decodedPath, _ := base64.StdEncoding.DecodeString(currentPath)

	// Search custom path for this file
	customPathDisplay := "-"
	filePath := filepath.Join(string(decodedPath), file.Name())
	customPath, exists := customPaths[filePath]
	if exists {
		customPathDisplay = html.EscapeString(customPath)
	}

	// Determine if the file is readable (text)
	isReadableFile := IsTextFile(file.Name())

	// Use action-buttons and appropriate button styles
	downloadLink := fmt.Sprintf(`<button class="action-btn download" title="Download" onclick="window.location.href='/download/?path=%s'"><i class="fa fa-download"></i></button>`, escapedencodedFilePath)
	deleteLink := fmt.Sprintf(`<button class="action-btn delete" title="Delete" onclick="window.location.href='/delete/?path=%s'"><i class="fa fa-trash"></i></button>`, escapedencodedFilePath)
	copyURLButton := fmt.Sprintf(`<button class="action-btn link" title="Copy URL" onclick="copyToClipboard('%s', '%s')"><i class="fa fa-link"></i></button>`, currentPath, escapedFileName)
	customPathButton := fmt.Sprintf(`<button class="action-btn edit" title="Create Custom Path" onclick="showCustomPathForm('%s', '%s')"><i class="fa fa-magic"></i></button>`, escapedFileName, currentPath)

	// Search button only for readable files
	searchButton := ""
	if isReadableFile {
		searchButton = fmt.Sprintf(`<button class="action-btn search" title="Search in File" onclick="showSearchModal('%s', '%s')"><i class="fa fa-search"></i></button>`, escapedencodedFilePath, escapedFileName)
	}

	fileSize, units := formatFileSize(fileInfo.Size())
	lastModified := fileInfo.ModTime().Format("2006-01-02 15:04:05")

	return fmt.Sprintf(`
		<tr>
			<td>%s</td>
			<td>%s</td>
			<td>%.2f %s</td>
			<td>%s</td>
			<td>%s</td>
			<td>
				<div class="action-buttons">
					%s%s%s%s%s
				</div>
			</td>
		</tr>
	`, escapedFileName, fileInfo.Mode(), fileSize, units, lastModified, customPathDisplay,
		downloadLink, copyURLButton, customPathButton, searchButton, deleteLink)
}

// CreateBackButton generates HTML for the back button
func CreateBackButton(currentPath string) string {
	if currentPath != "" {
		return `<button class="btn" onclick="window.location.href='/'"><i class="fa fa-arrow-left"></i> Back</button>`
	}
	return ""
}

// CreateZipButton generates HTML for the zip download button
func CreateZipButton(currentPath string) string {
	if currentPath != "" {
		return `<button class="btn" onclick="window.location.href='/zip?path=` + currentPath + `'"><i class="fa fa-download"></i> Download Zip</button>`
	} else {
		return `<button class="btn" onclick="window.location.href='/zip'"><i class="fa fa-download"></i> Download Zip</button>`
	}
}
