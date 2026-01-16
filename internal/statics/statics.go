package statics

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed templates css js
var staticFiles embed.FS

// Template es el template HTML principal
var indexTemplate *template.Template

// Estructura para los datos del template
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
		panic("Error al cargar el template: " + err.Error())
	}
}

// GetTemplates genera el HTML con los recursos embebidos
func GetTemplates(table string, backButton string, downloadButton string, disableHiddenFiles bool, readOnly bool) string {
	// Cargar CSS
	cssBytes, err := fs.ReadFile(staticFiles, "css/styles.css")
	if err != nil {
		panic("Error al leer CSS: " + err.Error())
	}

	// Ya no escapamos los porcentajes en CSS para evitar problemas de visualización
	cssString := string(cssBytes)

	// Cargar JavaScript
	jsBytes, err := fs.ReadFile(staticFiles, "js/main.js")
	if err != nil {
		panic("Error al leer JavaScript: " + err.Error())
	}

	// Configurar la variable de mostrar archivos ocultos
	hiddenDisplay := "display: flex;"
	if disableHiddenFiles {
		hiddenDisplay = "display: none;"
	}

	// Preparar datos para el template
	data := TemplateData{
		CSS:            template.CSS(cssString),
		Table:          template.HTML(table),
		BackButton:     template.HTML(backButton),
		DownloadButton: template.HTML(downloadButton),
		HiddenDisplay:  hiddenDisplay,
		ReadOnlyMode:   readOnly,
		JavaScript:     template.JS(string(jsBytes)),
	}

	// Renderizar a un string
	builder := &strings.Builder{}
	if err := indexTemplate.Execute(builder, data); err != nil {
		panic("Error al renderizar template: " + err.Error())
	}

	return builder.String()
}
