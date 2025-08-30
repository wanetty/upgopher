var input = document.getElementById('file-upload');

// Función para escapar HTML y prevenir XSS
function escapeHtml(str) {
    // Previene ataques XSS escapando caracteres especiales
    if (str === null || str === undefined) {
        return '';
    }
    
    // Asegurarnos de que es string
    str = String(str);
    
    // Escapar caracteres peligrosos
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;')
        .replace(/\//g, '&#x2F;')
        .replace(/\\/g, '&#x5C;')
        .replace(/`/g, '&#96;');
}
    
// Tab functionality
function openTab(evt, tabName) {
    var i, tabcontent, tablinks;
    
    // Hide all tab content
    tabcontent = document.getElementsByClassName("tab-content");
    for (i = 0; i < tabcontent.length; i++) {
        tabcontent[i].classList.remove("active");
    }
    
    // Remove "active" class from all tab buttons
    tablinks = document.getElementsByClassName("tab-link");
    for (i = 0; i < tablinks.length; i++) {
        tablinks[i].classList.remove("active");
    }
    
    // Show the current tab and add "active" class to the button that opened the tab
    document.getElementById(tabName).classList.add("active");
    evt.currentTarget.classList.add("active");
}

document.addEventListener('DOMContentLoaded', function() {
    const checkbox = document.getElementById('showAlertCheckbox');
    const clipboardTextarea = document.getElementById('shared-clipboard-textarea');

    // Load shared clipboard content when page loads
    fetch('/clipboard')
        .then(response => {
            if (!response.ok) {
                throw new Error('Error loading shared clipboard');
            }
            return response.text();
        })
        .then(data => {
            console.log('Clipboard loaded:', data);
            clipboardTextarea.value = data;
        })
        .catch(error => {
            console.error('Error loading shared clipboard:', error);
        });

    // Code for hidden files handling
    fetch('/showhiddenfiles')
        .then(response => response.json())
        .then(data => {
            if(data === true) {
                checkbox.checked = true;
            } else {
                checkbox.checked = false;
            }
        })
        .catch(error => {
            console.error('Error fetching showHiddenFiles status:', error);
        });

    checkbox.addEventListener('change', function() {
        fetch('/showhiddenfiles', {
            method: 'POST',
        })
        .then(data => {
            window.location.reload();
        })
        .catch(error => {
            console.error('Error:', error);
        });
    });

    // Añadir lógica para mostrar el nombre del archivo seleccionado
    const fileInput = document.getElementById('file-upload'); // Corregido el ID
    const fileNameDisplay = document.getElementById('file-name'); // Corregido el ID

    if (fileInput && fileNameDisplay) {
        fileInput.addEventListener('change', function() {
            if (this.files && this.files.length > 0) {
                fileNameDisplay.textContent = this.files[0].name;
            } else {
                fileNameDisplay.textContent = ''; // Limpiar si no hay archivo seleccionado
            }
        });
    }

    // Initialize file upload with progress
    setupFileUpload();
});

// Function to save text to shared clipboard
function saveToSharedClipboard() {
    const clipboardText = document.getElementById('shared-clipboard-textarea').value;
    console.log('Saving to clipboard:', clipboardText);
    
    fetch('/clipboard', {
        method: 'POST',
        headers: {
            'Content-Type': 'text/plain'
        },
        body: clipboardText
    })
    .then(response => {
        if (!response.ok) {
            throw new Error('Error saving to shared clipboard');
        }
        console.log('Clipboard saved successfully');
        alert('Text saved to shared clipboard successfully');
    })
    .catch(error => {
        console.error('Error saving to clipboard:', error);
        alert('Error saving to shared clipboard: ' + error.message);
    });
}

// Function to copy text from shared clipboard to local clipboard
function copyFromSharedClipboard() {
    const clipboardText = document.getElementById('shared-clipboard-textarea').value;
    
    if (!clipboardText) {
        alert('No text to copy');
        return;
    }
    
    navigator.clipboard.writeText(clipboardText)
        .then(() => {
            console.log('Text copied to local clipboard');
            alert('Text copied to local clipboard');
        })
        .catch(err => {
            console.error('Error copying text: ', err);
            
            // Alternative implementation for browsers that don't support navigator.clipboard
            try {
                const textArea = document.createElement('textarea');
                textArea.value = clipboardText;
                textArea.style.position = 'fixed';
                textArea.style.left = '-999999px';
                textArea.style.top = '-999999px';
                document.body.appendChild(textArea);
                textArea.focus();
                textArea.select();
                const successful = document.execCommand('copy');
                document.body.removeChild(textArea);
                
                if (successful) {
                    alert('Text copied to local clipboard');
                } else {
                    alert('Could not copy to clipboard');
                }
            } catch (err) {
                alert('Could not copy to clipboard: ' + err.message);
            }
        });
}

// Other functions...
function dropHandler(ev) {
    ev.preventDefault();
    if (ev.dataTransfer.items) {
        [...ev.dataTransfer.items].forEach((item, i) => {
            if (item.kind === "file") {
                const file = item.getAsFile();
                if (file) {
                    input.files = ev.dataTransfer.files;
                    // Update file name display
                    const fileNameDisplay = document.getElementById('file-name');
                    if (fileNameDisplay) {
                        fileNameDisplay.textContent = file.name;
                    }
                    // Trigger upload
                    uploadFile(file);
                }
            }
        });
    } else {
        [...ev.dataTransfer.files].forEach((file, i) => {
            input.files = ev.dataTransfer.files;
            // Update file name display
            const fileNameDisplay = document.getElementById('file-name');
            if (fileNameDisplay && file) {
                fileNameDisplay.textContent = file.name;
            }
            // Trigger upload
            if (file) {
                uploadFile(file);
            }
        });
    }
}

function dragOverHandler(ev) {
    ev.preventDefault();
}

// Function to handle file upload (used by both form submit and drag & drop)
function uploadFile(file) {
    uploadTotalSize = file.size;
    uploadStartTime = Date.now();
    
    const formData = new FormData();
    formData.append('file', file);
    
    showUploadProgress();
    
    // Create XMLHttpRequest for progress tracking
    const xhr = new XMLHttpRequest();
    
    // Upload progress event
    xhr.upload.addEventListener('progress', function(e) {
        if (e.lengthComputable) {
            const percentage = (e.loaded / e.total) * 100;
            updateUploadProgress(e.loaded, e.total, percentage);
        }
    });
    
    // Upload complete event
    xhr.addEventListener('load', function() {
        if (xhr.status === 200) {
            updateUploadProgress(uploadTotalSize, uploadTotalSize, 100);
            setTimeout(() => {
                hideUploadProgress();
                // Clear file input
                const fileInput = document.getElementById('file-upload');
                if (fileInput) {
                    fileInput.value = '';
                }
                const fileNameDisplay = document.getElementById('file-name');
                if (fileNameDisplay) {
                    fileNameDisplay.textContent = '';
                }
                // Reload page to show new file
                window.location.reload();
            }, 1500);
        } else {
            hideUploadProgress();
            alert('Error al subir el archivo: ' + xhr.statusText);
        }
    });
    
    // Upload error event
    xhr.addEventListener('error', function() {
        hideUploadProgress();
        alert('Error al subir el archivo');
    });
    
    // Upload abort event
    xhr.addEventListener('abort', function() {
        hideUploadProgress();
        alert('Subida de archivo cancelada');
    });
    
    // Send the request
    xhr.open('POST', '/');
    xhr.send(formData);
}

function copyToClipboard(pathBase64, fileName) {
    const decodedPath = atob(pathBase64);
    const baseUrl = window.location.origin;
    
    // Create a clean path without duplicate slashes
    let path = decodedPath;
    
    // Remove leading slashes in decodedPath if they exist
    while (path.startsWith('/')) {
        path = path.substring(1);
    }
    
    // Ensure slash between path and fileName
    if (path) {
        path = path.endsWith('/') ? path + fileName : path + '/' + fileName;
    } else {
        path = fileName;
    }
    
    // Create the final URL ensuring there's only one slash after /raw/
    const urlWithParam = baseUrl + "/raw/" + path;
    
    navigator.clipboard.writeText(urlWithParam);
}

function showCustomPathForm(fileName, currentPath) {
    const originalPath = currentPath ? atob(currentPath) + "/" + fileName : fileName;
    document.getElementById('originalPath').value = originalPath;
    document.getElementById('customPath').value = '';
    document.getElementById('customPathModal').style.display = 'flex';
    document.body.style.overflow = 'hidden';
}

function closeCustomPathModal() {
    document.getElementById('customPathModal').style.display = 'none';
    document.body.style.overflow = 'auto';
}

function createCustomPath() {
    const originalPath = document.getElementById('originalPath').value;
    const customPath = document.getElementById('customPath').value;

    if (!customPath) {
        alert('Please enter a custom path');
        return;
    }

    fetch('/custom-path', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: 'originalPath=' + encodeURIComponent(originalPath) + '&customPath=' + encodeURIComponent(customPath)
    })
    .then(response => {
        if (!response.ok) {
            throw new Error('Invalid path, already used or incompatible format');
        }
        return response.text();
    })
    .then(result => {
        alert('Custom path created successfully!\nAccess your file at: ' + window.location.origin + '/' + customPath);
        closeCustomPathModal();
        window.location.reload();
    })
    .catch(error => {
        alert('Error creating custom path: ' + error.message);
    });
}

// Close modal when clicking outside of it
window.onclick = function(event) {
    if (event.target == document.getElementById('customPathModal')) {
        closeCustomPathModal();
    }
    if (event.target == document.getElementById('searchModal')) {
        closeSearchModal();
    }
}

// Close modal with Escape key
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        closeCustomPathModal();
        closeSearchModal();
    }
});

// Search in file functionality
let currentFilePath = '';

function showSearchModal(filePath, fileName) {
    // Validar los parámetros antes de usarlos
    if (!filePath || typeof filePath !== 'string') {
        console.error('Error: filePath inválido en showSearchModal');
        return;
    }
    
    // Validar que fileName sea una cadena
    if (!fileName || typeof fileName !== 'string') {
        fileName = 'archivo';
        console.error('Error: fileName inválido en showSearchModal');
    }
    
    // Guardar el path encodado tal cual viene
    currentFilePath = filePath;
    
    // Asegurarse de que el nombre del archivo se muestra de forma segura
    document.getElementById('searchFileName').textContent = escapeHtml(fileName);
    
    document.getElementById('searchTerm').value = '';
    document.getElementById('searchResults').innerHTML = '<div class="placeholder-text">Enter a search term above to find matches</div>';
    document.getElementById('resultCount').textContent = '(0)';
    document.getElementById('searchModal').style.display = 'flex';
    document.body.style.overflow = 'hidden';
    
    const searchInput = document.getElementById('searchTerm');
    
    // Limpiar eventos previos para evitar duplicados
    const newSearchInput = searchInput.cloneNode(true);
    searchInput.parentNode.replaceChild(newSearchInput, searchInput);
    
    // Añadir evento de tecla Enter para iniciar la búsqueda
    newSearchInput.addEventListener('keypress', function(event) {
        if (event.key === 'Enter') {
            event.preventDefault();
            searchInFile();
        }
    });
    
    setTimeout(() => newSearchInput.focus(), 100);
    
    // Para depuración - no usar decodeURIComponent aquí pues puede romper
    // la codificación en base64
    console.log('Path para búsqueda (no decodificado):', filePath);
}

function closeSearchModal() {
    document.getElementById('searchModal').style.display = 'none';
    document.body.style.overflow = 'auto';
    
    // Limpiar variables para liberar memoria y evitar posibles fugas de seguridad
    currentFilePath = '';
    document.getElementById('searchTerm').value = '';
    document.getElementById('searchResults').innerHTML = '';
}

function searchInFile() {
    const searchTerm = document.getElementById('searchTerm').value;
    const caseSensitive = document.getElementById('caseSensitive').checked;
    const wholeWord = document.getElementById('wholeWord').checked;
    
    // Validar que tenemos un término de búsqueda
    if (!searchTerm) {
        document.getElementById('searchResults').innerHTML = '<div class="placeholder-text">Enter a search term above to find matches</div>';
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }
    
    // Validar que tenemos una ruta de archivo válida
    if (!currentFilePath || typeof currentFilePath !== 'string') {
        document.getElementById('searchResults').innerHTML = '<div class="error-message">Ruta de archivo inválida</div>';
        document.getElementById('resultCount').textContent = '(0)';
        console.error('Error: currentFilePath inválido en searchInFile');
        return;
    }
    
    // Validar el término de búsqueda (no permitir términos muy largos o peligrosos)
    if (searchTerm.length > 1000) {
        document.getElementById('searchResults').innerHTML = '<div class="error-message">El término de búsqueda es demasiado largo</div>';
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }
    
    // Mostrar indicador de carga
    document.getElementById('searchResults').innerHTML = '<div class="loading-results">Searching...</div>';
    
    // Para la búsqueda, utilizamos el path tal cual lo recibimos sin ninguna codificación adicional
    // Solo aplicamos encodeURIComponent al término de búsqueda y otros parámetros
    const url = `/search-file?path=${encodeURIComponent(currentFilePath)}&term=${encodeURIComponent(searchTerm)}&caseSensitive=${encodeURIComponent(caseSensitive)}&wholeWord=${encodeURIComponent(wholeWord)}`;
    
    // Log extensivo para depuración
    console.log('Ruta original para búsqueda:', currentFilePath);
    
    console.log('Enviando solicitud a:', url);
    
    // Establecer un tiempo de espera para la solicitud
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 segundos timeout
    
    fetch(url, { signal: controller.signal })
        .then(response => {
            clearTimeout(timeoutId);
            
            if (!response.ok) {
                console.error('Error HTTP:', response.status, response.statusText);
                return response.text().then(text => {
                    throw new Error(`Error en la búsqueda (${response.status}): ${text}`);
                });
            }
            return response.json();
        })
        .then(results => {
            // Verificar que los resultados tienen el formato esperado
            if (!Array.isArray(results)) {
                throw new Error('Formato de respuesta inválido');
            }
            
            console.log('Resultados recibidos:', results);
            displaySearchResults(searchTerm, results);
        })
        .catch(error => {
            clearTimeout(timeoutId);
            console.error('Error completo:', error);
            
            // Preparar un mensaje de error seguro (escapado)
            const errorMsg = error.name === 'AbortError' 
                ? 'La búsqueda tardó demasiado tiempo y se canceló'
                : `Ha ocurrido un error durante la búsqueda: ${escapeHtml(error.message)}`;
                
            document.getElementById('searchResults').innerHTML = 
                `<div class="error-message">${errorMsg}</div>`;
            document.getElementById('resultCount').textContent = '(0)';
        });
}

function displaySearchResults(searchTerm, results) {
    const resultsContainer = document.getElementById('searchResults');
    
    if (results.length === 0) {
        resultsContainer.innerHTML = '<div class="no-results">No matches found</div>';
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }
    
    // Verificar si hay un mensaje especial (lineNumber === -1)
    if (results.length === 1 && results[0].lineNumber === -1) {
        // Asegurar que el contenido esté siempre escapado para prevenir XSS
        const safeContent = escapeHtml(results[0].content);
        if (safeContent.includes("No se encontraron")) {
            resultsContainer.innerHTML = '<div class="no-results">' + safeContent + '</div>';
        } else {
            resultsContainer.innerHTML = '<div class="info-message">' + safeContent + '</div>';
        }
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }
    
    document.getElementById('resultCount').textContent = `(${results.length})`;
    
    // Crear HTML para los resultados
    let htmlContent = '';
    results.forEach(result => {
        // Saltar mensajes especiales que pudieran estar al final
        if (result.lineNumber === -1) {
            // Asegurar que el contenido especial esté escapado
            htmlContent += `<div class="info-message">${escapeHtml(result.content)}</div>`;
            return;
        }
        
        // Escapar el número de línea por seguridad (aunque debería ser un número)
        const safeLineNumber = typeof result.lineNumber === 'number' ? 
            result.lineNumber : escapeHtml(String(result.lineNumber));
        
        // Resaltar términos de búsqueda en el contenido (asegurándose que está escapado)
        const highlightedContent = highlightSearchTerm(result.content, searchTerm);
        
        htmlContent += `
            <div class="search-result-item">
                <span class="result-line-number">${safeLineNumber}</span>
                <div class="result-line-content">${highlightedContent}</div>
            </div>
        `;
    });
    
    resultsContainer.innerHTML = htmlContent;
}

function highlightSearchTerm(text, term) {
    // Validar parámetros
    if (!text || typeof text !== 'string') {
        console.error('Error: texto inválido en highlightSearchTerm');
        return '';
    }
    
    if (!term || typeof term !== 'string') {
        console.error('Error: término de búsqueda inválido en highlightSearchTerm');
        return escapeHtml(text);
    }
    
    // Primero escapamos el texto para prevenir XSS
    const safeText = escapeHtml(text);
    
    // Escape caracteres especiales en regex
    const escapedTerm = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    
    try {
        const regex = new RegExp(`(${escapedTerm})`, 'gi');
        return safeText.replace(regex, (match) => `<span class="result-match">${match}</span>`);
    } catch (error) {
        console.error('Error al resaltar texto:', error);
        return safeText; // En caso de error, devolver el texto escapado
    }
}

// En una implementación real, esta función haría una solicitud al servidor
function fetchFileContent(filePath) {
    return new Promise((resolve, reject) => {
        fetch(`/file-content?path=${filePath}`)
            .then(response => {
                if (!response.ok) {
                    throw new Error('Error fetching file content');
                }
                return response.text();
            })
            .then(content => resolve(content))
            .catch(error => reject(error));
    });
}

function sortTable(n, type = 'string') {
    const table = document.querySelector(".styled-table tbody");
    const rows = Array.from(table.rows);
    const isAsc = table.dataset.sortOrder === 'asc';
    const sortOrder = isAsc ? 'desc' : 'asc';
    table.dataset.sortOrder = sortOrder;

    rows.sort((rowA, rowB) => {
        const cellA = rowA.cells[n].innerText.trim();
        const cellB = rowB.cells[n].innerText.trim();

        // Last Modified column (date sorting)
        if (n === 3) {
            // Try to parse as ISO date, fallback to string
            const dateA = Date.parse(cellA);
            const dateB = Date.parse(cellB);
            if (!isNaN(dateA) && !isNaN(dateB)) {
                return sortOrder === 'asc' ? dateA - dateB : dateB - dateA;
            }
            // Fallback to string compare if not valid date
            if (cellA < cellB) return sortOrder === 'asc' ? -1 : 1;
            if (cellA > cellB) return sortOrder === 'asc' ? 1 : -1;
            return 0;
        }
        if (type === 'number') {
            const numA = parseFloat(cellA) || 0;
            const numB = parseFloat(cellB) || 0;
            return sortOrder === 'asc' ? numA - numB : numB - numA;
        } else {
            if (cellA < cellB) return sortOrder === 'asc' ? -1 : 1;
            if (cellA > cellB) return sortOrder === 'asc' ? 1 : -1;
            return 0;
        }
    });

    table.innerHTML = "";
    rows.forEach(row => table.appendChild(row));

    document.querySelectorAll("th i").forEach(icon => icon.className = 'fa');
    const iconId = n === 0 ? 'name-icon' :
                    n === 2 ? 'size-icon' :
                    n === 3 ? 'modified-icon' :
                    n === 4 ? 'custom-path-icon' : '';
    if (iconId) {
        const icon = document.getElementById(iconId);
        icon.className = "fa fa-sort-" + (sortOrder === 'asc' ? 'asc' : 'desc');
    }
}

// Upload progress functionality
let uploadStartTime = 0;
let uploadTotalSize = 0;

function showUploadProgress() {
    const progressContainer = document.getElementById('upload-progress-container');
    const uploadForm = document.getElementById('upload-form');
    const uploadBtn = document.getElementById('upload-btn');
    
    progressContainer.style.display = 'block';
    uploadBtn.disabled = true;
    uploadBtn.value = 'Uploading...';
    
    // Reset progress
    updateUploadProgress(0, 0, 0);
}

function hideUploadProgress() {
    const progressContainer = document.getElementById('upload-progress-container');
    const uploadBtn = document.getElementById('upload-btn');
    
    setTimeout(() => {
        progressContainer.style.display = 'none';
        uploadBtn.disabled = false;
        uploadBtn.value = 'Upload';
    }, 1000);
}

function updateUploadProgress(loaded, total, percentage) {
    const progressFill = document.getElementById('upload-progress-fill');
    const progressPercentage = document.getElementById('upload-progress-percentage');
    const progressText = document.getElementById('upload-progress-text');
    const uploadSpeed = document.getElementById('upload-speed');
    const uploadEta = document.getElementById('upload-eta');
    
    // Update progress bar
    progressFill.style.width = percentage + '%';
    progressPercentage.textContent = Math.round(percentage) + '%';
    
    // Update progress text
    if (percentage === 100) {
        progressText.textContent = 'Upload completed!';
    } else if (percentage > 0) {
        progressText.textContent = 'Uploading...';
    }
    
    // Calculate and display speed and ETA
    if (loaded > 0 && uploadStartTime > 0) {
        const elapsedTime = (Date.now() - uploadStartTime) / 1000; // in seconds
        const speed = loaded / elapsedTime; // bytes per second
        const remainingBytes = total - loaded;
        const eta = remainingBytes / speed; // seconds
        
        // Format speed
        if (speed > 1024 * 1024) {
            uploadSpeed.textContent = (speed / (1024 * 1024)).toFixed(1) + ' MB/s';
        } else if (speed > 1024) {
            uploadSpeed.textContent = (speed / 1024).toFixed(1) + ' KB/s';
        } else {
            uploadSpeed.textContent = speed.toFixed(0) + ' B/s';
        }
        
        // Format ETA
        if (eta < 60) {
            uploadEta.textContent = 'ETA: ' + Math.round(eta) + 's';
        } else if (eta < 3600) {
            uploadEta.textContent = 'ETA: ' + Math.round(eta / 60) + 'm';
        } else {
            uploadEta.textContent = 'ETA: ' + Math.round(eta / 3600) + 'h';
        }
    } else {
        uploadSpeed.textContent = '0 KB/s';
        uploadEta.textContent = 'ETA: --';
    }
}

function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + sizes[i];
}

// Enhanced file upload with progress
function setupFileUpload() {
    const uploadForm = document.getElementById('upload-form');
    const fileInput = document.getElementById('file-upload');
    
    if (!uploadForm || !fileInput) {
        console.error('Upload form or file input not found');
        return;
    }
    
    uploadForm.addEventListener('submit', function(e) {
        e.preventDefault();
        
        const files = fileInput.files;
        if (!files || files.length === 0) {
            alert('Por favor selecciona un archivo');
            return;
        }
        
        const file = files[0];
        uploadFile(file);
    });
}