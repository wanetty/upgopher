package statics

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed templates css js
var staticFiles embed.FS

// indexTemplate is the main HTML template
var indexTemplate *template.Template

// TemplateData holds the data for the template
type TemplateData struct {
	CSS            template.CSS
	Table          template.HTML
	BackButton     template.HTML
	DownloadButton template.HTML
	HiddenDisplay  string
	ReadOnlyMode   bool
	JavaScript     template.JS
}

func init() {
	var err error
	indexTemplate, err = template.ParseFS(staticFiles, "templates/index.html")
	if err != nil {
		panic("Error loading template: " + err.Error())
	}
}

// GetTemplates generates HTML with embedded resources
func GetTemplates(table string, backButton string, downloadButton string, disableHiddenFiles bool, readOnly bool) string {
	// Load CSS
	cssBytes, err := fs.ReadFile(staticFiles, "css/styles.css")
	if err != nil {
		panic("Error reading CSS: " + err.Error())
	}

	// CSS percentages are not escaped to avoid display issues
	cssString := string(cssBytes)

	// Load JavaScript
	jsBytes, err := fs.ReadFile(staticFiles, "js/main.js")
	if err != nil {
		panic("Error reading JavaScript: " + err.Error())
	}

	// Configure hidden files display setting
	hiddenDisplay := "display: flex;"
	if disableHiddenFiles {
		hiddenDisplay = "display: none;"
	}

	// Prepare template data
	data := TemplateData{
		CSS:            template.CSS(cssString),
		Table:          template.HTML(table),
		BackButton:     template.HTML(backButton),
		DownloadButton: template.HTML(downloadButton),
		HiddenDisplay:  hiddenDisplay,
		ReadOnlyMode:   readOnly,
		JavaScript:     template.JS(string(jsBytes)),
	}

	// Render to string
	builder := &strings.Builder{}
	if err := indexTemplate.Execute(builder, data); err != nil {
		panic("Error rendering template: " + err.Error())
	}

	return builder.String()
}
