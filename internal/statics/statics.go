package statics

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed templates css js
var staticFiles embed.FS

// indexTemplate is the main HTML template
var indexTemplate *template.Template

// TemplateData holds the data for the template
type TemplateData struct {
	CSS              template.CSS
	Table            template.HTML
	CurrentPath      string
	DownloadButton   template.HTML
	HiddenDisplay    string
	ReadOnlyMode     bool
	JavaScript       template.JS
	CryptoJavaScript template.JS
	EncryptTransfers bool
}

func init() {
	var err error
	indexTemplate, err = template.ParseFS(staticFiles, "templates/index.html")
	if err != nil {
		panic("Error loading template: " + err.Error())
	}
}

// GetTemplates generates HTML with embedded resources
func GetTemplates(table string, currentPath string, downloadButton string, disableHiddenFiles bool, readOnly bool, encrypted ...bool) string {
	cssBytes, err := fs.ReadFile(staticFiles, "css/styles.css")
	if err != nil {
		panic("Error reading CSS: " + err.Error())
	}

	// CSS percentages are not escaped to avoid display issues
	cssString := string(cssBytes)

	jsBytes, err := fs.ReadFile(staticFiles, "js/main.js")
	if err != nil {
		panic("Error reading JavaScript: " + err.Error())
	}

	// Configure hidden files display setting
	hiddenDisplay := "display: flex;"
	if disableHiddenFiles {
		hiddenDisplay = "display: none;"
	}

	cryptoJS, _ := fs.ReadFile(staticFiles, "js/crypto.js")
	data := TemplateData{
		EncryptTransfers: len(encrypted) > 0 && encrypted[0],
		CryptoJavaScript: template.JS(string(cryptoJS)),
		CSS:              template.CSS(cssString),
		Table:            template.HTML(table),
		CurrentPath:      currentPath,
		DownloadButton:   template.HTML(downloadButton),
		HiddenDisplay:    hiddenDisplay,
		ReadOnlyMode:     readOnly,
		JavaScript:       template.JS(string(jsBytes)),
	}

	builder := &strings.Builder{}
	if err := indexTemplate.Execute(builder, data); err != nil {
		panic("Error rendering template: " + err.Error())
	}

	return builder.String()
}

// TransferPage serves a shell for shared links. The shell obtains and decrypts
// file bytes through the encrypted transport; it never embeds file content.
func TransferPage(w http.ResponseWriter, r *http.Request) {
	js, _ := fs.ReadFile(staticFiles, "js/crypto.js")
	target, _ := json.Marshal(r.URL.RequestURI())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	page := template.Must(template.New("transfer").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Encrypted transfer — Upgopher</title><body><h1>Encrypted file transfer</h1><p>Protects against passive tunnel observers. The page and its JavaScript must be trusted.</p><button id="receive">Download file</button><p id="status" role="status"></p><a href="/">File manager</a><script>window.upgopherEncrypted=true;{{.Script}}document.getElementById('receive').onclick=function(){UpgopherCrypto.download({{.Target}}).catch(function(e){document.getElementById('status').textContent=e.message;});};</script></body></html>`))
	_ = page.Execute(w, struct {
		Script template.JS
		Target template.JS
	}{template.JS(string(js)), template.JS(target)})
}
