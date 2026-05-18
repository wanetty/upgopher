var input = document.getElementById('file-upload');

// Function to escape HTML and prevent XSS
function escapeHtml(str) {
    // Prevents XSS attacks by escaping special characters
    if (str === null || str === undefined) {
        return '';
    }

    // Ensure it's a string
    str = String(str);

    // Escape dangerous characters
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

// ── Breadcrumb navigation ─────────────────────────────────────────────────────

/**
 * Fetches path segments from the server and renders a clickable breadcrumb
 * bar above the file table. Middle segments are collapsed to '…' on narrow
 * screens via CSS (max-width / overflow hidden + ellipsis on .breadcrumb-segment).
 */
function loadBreadcrumbs() {
    var bar = document.getElementById('breadcrumb-bar');
    if (!bar) return;

    var pathInput = document.getElementById('current-path-value');
    var currentPath = pathInput ? pathInput.value : '';

    // Always show the home crumb
    var homeHtml = '<a class="breadcrumb-segment" href="/" title="Root"><i class="fa fa-home"></i></a>';

    if (!currentPath) {
        bar.innerHTML = homeHtml + '<span class="breadcrumb-segment active">Root</span>';
        return;
    }

    fetch('/api/v1/breadcrumbs?path=' + encodeURIComponent(currentPath))
        .then(function (r) {
            if (!r.ok) throw new Error('breadcrumbs fetch failed');
            return r.json();
        })
        .then(function (data) {
            var html = homeHtml;
            var segments = data.segments || [];
            segments.forEach(function (seg, i) {
                html += '<span class="breadcrumb-separator" aria-hidden="true">›</span>';
                if (i === segments.length - 1) {
                    // Current directory — not a link
                    html += '<span class="breadcrumb-segment active">' + escapeHtml(seg.name) + '</span>';
                } else {
                    html += '<a class="breadcrumb-segment" href="/?path=' + encodeURIComponent(seg.path) + '">' + escapeHtml(seg.name) + '</a>';
                }
            });
            bar.innerHTML = html;
        })
        .catch(function () {
            // On any error just show the home link — fail silently
            bar.innerHTML = homeHtml;
        });
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

document.addEventListener('DOMContentLoaded', function () {
    const checkbox = document.getElementById('showAlertCheckbox');

    // Load clipboard tabs on page load
    loadClipboardTabs();

    // Initialize auto-save (SSE + debounce + toggle)
    initAutoSave();

    // Restore file selection state from sessionStorage
    initCheckboxes();

    // Render the breadcrumb navigation bar
    loadBreadcrumbs();

    // Update char count when textarea changes
    var clipboardTextarea = document.getElementById('shared-clipboard-textarea');
    if (clipboardTextarea) {
        clipboardTextarea.addEventListener('input', function () {
            document.getElementById('clipboard-char-count').textContent = 'chars: ' + this.value.length;
        });
    }

    // New tab name: create on Enter, cancel on Escape
    var newTabInput = document.getElementById('new-tab-name');
    if (newTabInput) {
        newTabInput.addEventListener('keydown', function (e) {
            if (e.key === 'Enter') { e.preventDefault(); createClipboardTab(); }
            if (e.key === 'Escape') hideNewTabInput();
        });
    }

    // Token unlock: submit on Enter, cancel on Escape
    var unlockInput = document.getElementById('token-unlock-input');
    if (unlockInput) {
        unlockInput.addEventListener('keydown', function (e) {
            if (e.key === 'Enter') { e.preventDefault(); submitTabToken(); }
            if (e.key === 'Escape') hideTokenUnlockRow();
        });
    }

    // Code for hidden files handling
    fetch('/showhiddenfiles')
        .then(response => response.json())
        .then(data => {
            if (data === true) {
                checkbox.checked = true;
            } else {
                checkbox.checked = false;
            }
        })
        .catch(error => {
            console.error('Error fetching showHiddenFiles status:', error);
        });

    checkbox.addEventListener('change', function () {
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

    // Add logic to display the selected file name
    const fileInput = document.getElementById('file-upload');
    const fileNameDisplay = document.getElementById('file-name');

    if (fileInput && fileNameDisplay) {
        fileInput.addEventListener('change', function () {
            if (this.files && this.files.length > 0) {
                fileNameDisplay.textContent = this.files[0].name;
            } else {
                fileNameDisplay.textContent = ''; // Clear if no file selected
            }
        });
    }

    // Initialize file upload with progress
    setupFileUpload();
});

// Function to save text to shared clipboard
function saveToSharedClipboard(isAutoSave) {
    var clipboardText = document.getElementById('shared-clipboard-textarea').value;
    var headers = { 'Content-Type': 'text/plain' };
    var token = clipboardTokenCache[currentClipboardTab];
    if (token) headers['X-Tab-Token'] = token;

    if (isAutoSave) setAutoSaveStatus('saving');

    fetch('/clipboard?tab=' + encodeURIComponent(currentClipboardTab), {
        method: 'POST',
        headers: headers,
        body: clipboardText
    })
        .then(function (response) {
            if (response.status === 401) {
                setClipboardEditable(false);
                showTokenUnlockRow(currentClipboardTab, function () { saveToSharedClipboard(isAutoSave); });
                return;
            }
            if (!response.ok) throw new Error('Error saving to shared clipboard');
            _isLocallyDirty = false; // save succeeded — SSE updates can be applied again
            if (isAutoSave) {
                setAutoSaveStatus('saved');
                loadClipboardTabs(false);
            } else {
                showToast('Saved to "' + currentClipboardTab + '"');
                loadClipboardTabs(false);
            }
        })
        .catch(function (error) {
            if (isAutoSave) {
                setAutoSaveStatus('idle');
            }
            showToast(error.message, 'error');
        });
}

// Function to copy text from shared clipboard to local clipboard
function copyFromSharedClipboard() {
    var clipboardText = document.getElementById('shared-clipboard-textarea').value;

    if (!clipboardText) {
        showToast('Nothing to copy', 'error');
        return;
    }

    navigator.clipboard.writeText(clipboardText)
        .then(function () { showToast('Copied to local clipboard'); })
        .catch(function () {
            try {
                var textArea = document.createElement('textarea');
                textArea.value = clipboardText;
                textArea.style.position = 'fixed';
                textArea.style.left = '-999999px';
                textArea.style.top = '-999999px';
                document.body.appendChild(textArea);
                textArea.focus();
                textArea.select();
                var successful = document.execCommand('copy');
                document.body.removeChild(textArea);
                if (successful) {
                    showToast('Copied to local clipboard');
                } else {
                    showToast('Could not copy to clipboard', 'error');
                }
            } catch (err) {
                showToast('Could not copy: ' + err.message, 'error');
            }
        });
}

// Other functions...
function dropHandler(ev) {
    ev.preventDefault();

    if (ev.dataTransfer.items) {
        var promises = [];
        var firefoxWarning = false;
        for (var i = 0; i < ev.dataTransfer.items.length; i++) {
            if (ev.dataTransfer.items[i].kind === 'file') {
                var entry = ev.dataTransfer.items[i].webkitGetAsEntry();
                if (entry) {
                    promises.push(traverseEntry(entry, ''));
                } else {
                    // Fallback for browsers without webkitGetAsEntry
                    firefoxWarning = true;
                    var file = ev.dataTransfer.items[i].getAsFile();
                    if (file) {
                        promises.push(Promise.resolve([{
                            file: file,
                            relativePath: file.name
                        }]));
                    }
                }
            }
        }
        Promise.all(promises).then(function(results) {
            var allItems = results.flat();
            var fileItems = [];
            var emptyDirs = [];
            for (var k = 0; k < allItems.length; k++) {
                if (allItems[k]._emptyDir) {
                    emptyDirs.push(allItems[k].relativePath);
                } else {
                    fileItems.push(allItems[k]);
                }
            }
            var fileNameDisplay = document.getElementById('file-name');
            if (fileNameDisplay) {
                fileNameDisplay.textContent = fileItems.length + ' file(s)';
            }
            if (fileItems.length > 0 || emptyDirs.length > 0) {
                uploadFiles(fileItems, emptyDirs);
            }
            if (firefoxWarning) {
                showToast('Folder structure not preserved. Use "Choose Folder" button instead.', 'warning');
            }
        });
    } else {
        // Fallback for very old browsers: use dataTransfer.files
        var files = ev.dataTransfer.files;
        if (files && files.length > 0) {
            var fileItems = [];
            for (var j = 0; j < files.length; j++) {
                fileItems.push({
                    file: files[j],
                    relativePath: files[j].name
                });
            }
            var fileNameDisplay = document.getElementById('file-name');
            if (fileNameDisplay) {
                fileNameDisplay.textContent = fileItems.length + ' file(s)';
            }
            uploadFiles(fileItems, []);
        }
    }
}

function dragOverHandler(ev) {
    ev.preventDefault();
}

// Recursively traverse a FileSystemEntry (file or directory) and collect
// all files with their relative paths. Uses entry.fullPath as the canonical
// path source (supported in Chrome, Edge, Safari), falling back to manual
// construction via the path parameter.
function traverseEntry(entry, path) {
    return new Promise(function(resolve) {
        if (entry.isFile) {
            entry.file(function(file) {
                var relPath = (entry.fullPath && entry.fullPath.length > 1)
                    ? entry.fullPath.slice(1)
                    : (path + file.name);
                resolve([{ file: file, relativePath: relPath }]);
            }, function() {
                resolve([]);
            });
        } else if (entry.isDirectory) {
            var reader = entry.createReader();
            var allEntries = [];
            (function readBatch() {
                reader.readEntries(function(entries) {
                    if (entries.length === 0) {
                        if (allEntries.length === 0) {
                            var dirPath = (entry.fullPath && entry.fullPath.length > 1)
                                ? entry.fullPath.slice(1) + '/'
                                : path + entry.name + '/';
                            resolve([{ _emptyDir: true, relativePath: dirPath }]);
                        } else {
                            resolve(Promise.all(
                                allEntries.map(function(e) {
                                    return traverseEntry(e, path + entry.name + '/');
                                })
                            ).then(function(results) {
                                return results.flat();
                            }));
                        }
                    } else {
                        allEntries.push.apply(allEntries, entries);
                        readBatch();
                    }
                }, function() {
                    resolve([]);
                });
            })();
        } else {
            resolve([]);
        }
    });
}

// Single-file upload (kept for backward compatibility, delegates to batch)
function uploadFile(file) {
    uploadFiles([{ file: file, relativePath: file.name }], []);
}

// Batch upload: sends all files in a single multipart request.
function uploadFiles(fileItems, emptyDirs) {
    if (!emptyDirs) emptyDirs = [];
    if ((!fileItems || fileItems.length === 0) && emptyDirs.length === 0) return;

    // Calculate total size for progress tracking
    uploadTotalSize = 0;
    for (var i = 0; i < fileItems.length; i++) {
        uploadTotalSize += fileItems[i].file.size;
    }
    uploadStartTime = Date.now();

    var formData = new FormData();
    for (var j = 0; j < fileItems.length; j++) {
        var item = fileItems[j];
        formData.append('file', item.file, item.relativePath);
    }
    for (var d = 0; d < emptyDirs.length; d++) {
        formData.append('empty-dir', new Blob([]), emptyDirs[d]);
    }

    showUploadProgress();

    var xhr = new XMLHttpRequest();

    xhr.upload.addEventListener('progress', function(e) {
        if (e.lengthComputable) {
            var percentage = (e.loaded / e.total) * 100;
            updateUploadProgress(e.loaded, e.total, percentage);
        }
    });

    xhr.addEventListener('load', function() {
        if (xhr.status === 200 || xhr.status === 201) {
            updateUploadProgress(uploadTotalSize, uploadTotalSize, 100);
            setTimeout(function() {
                hideUploadProgress();
                var fileInput = document.getElementById('file-upload');
                if (fileInput) fileInput.value = '';
                var folderInput = document.getElementById('folder-upload');
                if (folderInput) folderInput.value = '';
                var fileNameDisplay = document.getElementById('file-name');
                if (fileNameDisplay) fileNameDisplay.textContent = '';
                window.location.reload();
            }, 1500);
        } else {
            hideUploadProgress();
            alert('Error al subir: ' + xhr.statusText);
        }
    });

    xhr.addEventListener('error', function() {
        hideUploadProgress();
        alert('Error al subir');
    });

    xhr.addEventListener('abort', function() {
        hideUploadProgress();
        alert('Subida cancelada');
    });

    var uploadUrl = '/' + window.location.search;
    xhr.open('POST', uploadUrl);
    xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
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
window.onclick = function (event) {
    if (event.target == document.getElementById('customPathModal')) {
        closeCustomPathModal();
    }
    if (event.target == document.getElementById('searchModal')) {
        closeSearchModal();
    }
    if (event.target == document.getElementById('fileViewerModal')) {
        closeFileViewer();
    }
    if (event.target == document.getElementById('newFolderModal')) {
        closeNewFolderModal();
    }
    if (event.target == document.getElementById('errorModal')) {
        closeErrorModal();
    }
}

// Close modal with Escape key
document.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') {
        closeCustomPathModal();
        closeSearchModal();
        closeFileViewer();
        closeNewFolderModal();
        closeErrorModal();
        closeTreePanel();
    }
    if (event.key === 'Enter' && document.getElementById('newFolderModal').style.display === 'flex') {
        event.preventDefault();
        createFolder();
    }
});

// Search in file functionality
let currentFilePath = '';

function showSearchModal(filePath, fileName) {
    // Validate parameters before using them
    if (!filePath || typeof filePath !== 'string') {
        console.error('Error: invalid filePath in showSearchModal');
        return;
    }

    // Validate that fileName is a string
    if (!fileName || typeof fileName !== 'string') {
        fileName = 'file';
        console.error('Error: invalid fileName in showSearchModal');
    }

    // Store the encoded path as received
    currentFilePath = filePath;

    // Ensure the file name is displayed safely
    document.getElementById('searchFileName').textContent = escapeHtml(fileName);

    document.getElementById('searchTerm').value = '';
    document.getElementById('searchResults').innerHTML = '<div class="placeholder-text">Enter a search term above to find matches</div>';
    document.getElementById('resultCount').textContent = '(0)';
    document.getElementById('searchModal').style.display = 'flex';
    document.body.style.overflow = 'hidden';

    const searchInput = document.getElementById('searchTerm');

    // Clear previous events to avoid duplicates
    const newSearchInput = searchInput.cloneNode(true);
    searchInput.parentNode.replaceChild(newSearchInput, searchInput);

    // Add Enter key event to start search
    newSearchInput.addEventListener('keypress', function (event) {
        if (event.key === 'Enter') {
            event.preventDefault();
            searchInFile();
        }
    });

    setTimeout(() => newSearchInput.focus(), 100);

    // For debugging - do not use decodeURIComponent here as it may break
    // the base64 encoding
    console.log('Search path (not decoded):', filePath);
}

function closeSearchModal() {
    document.getElementById('searchModal').style.display = 'none';
    document.body.style.overflow = 'auto';

    // Clear variables to free memory and prevent potential security leaks
    currentFilePath = '';
    document.getElementById('searchTerm').value = '';
    document.getElementById('searchResults').innerHTML = '';
}

function searchInFile() {
    const searchTerm = document.getElementById('searchTerm').value;
    const caseSensitive = document.getElementById('caseSensitive').checked;
    const wholeWord = document.getElementById('wholeWord').checked;

    // Validate that we have a search term
    if (!searchTerm) {
        document.getElementById('searchResults').innerHTML = '<div class="placeholder-text">Enter a search term above to find matches</div>';
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }

    // Validate that we have a valid file path
    if (!currentFilePath || typeof currentFilePath !== 'string') {
        document.getElementById('searchResults').innerHTML = '<div class="error-message">Invalid file path</div>';
        document.getElementById('resultCount').textContent = '(0)';
        console.error('Error: invalid currentFilePath in searchInFile');
        return;
    }

    // Validate search term (don't allow very long or dangerous terms)
    if (searchTerm.length > 1000) {
        document.getElementById('searchResults').innerHTML = '<div class="error-message">Search term is too long</div>';
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }

    // Show loading indicator
    document.getElementById('searchResults').innerHTML = '<div class="loading-results">Searching...</div>';

    // For search, use the path as received without additional encoding
    // Only apply encodeURIComponent to the search term and other parameters
    const url = `/search-file?path=${encodeURIComponent(currentFilePath)}&term=${encodeURIComponent(searchTerm)}&caseSensitive=${encodeURIComponent(caseSensitive)}&wholeWord=${encodeURIComponent(wholeWord)}`;

    // Extensive logging for debugging
    console.log('Original search path:', currentFilePath);

    console.log('Sending request to:', url);

    // Set request timeout
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 seconds timeout

    fetch(url, { signal: controller.signal })
        .then(response => {
            clearTimeout(timeoutId);

            if (!response.ok) {
                console.error('Error HTTP:', response.status, response.statusText);
                return response.text().then(text => {
                    throw new Error(`Search error (${response.status}): ${text}`);
                });
            }
            return response.json();
        })
        .then(results => {
            // Verify results have expected format
            if (!Array.isArray(results)) {
                throw new Error('Invalid response format');
            }

            console.log('Results received:', results);
            displaySearchResults(searchTerm, results);
        })
        .catch(error => {
            clearTimeout(timeoutId);
            console.error('Complete error:', error);

            // Prepare safe error message (escaped)
            const errorMsg = error.name === 'AbortError'
                ? 'Search took too long and was cancelled'
                : `An error occurred during search: ${escapeHtml(error.message)}`;

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

    // Check if there's a special message (lineNumber === -1)
    if (results.length === 1 && results[0].lineNumber === -1) {
        // Ensure content is always escaped to prevent XSS
        const safeContent = escapeHtml(results[0].content);
        if (safeContent.includes("No matches")) {
            resultsContainer.innerHTML = '<div class="no-results">' + safeContent + '</div>';
        } else {
            resultsContainer.innerHTML = '<div class="info-message">' + safeContent + '</div>';
        }
        document.getElementById('resultCount').textContent = '(0)';
        return;
    }

    document.getElementById('resultCount').textContent = `(${results.length})`;

    // Create HTML for results
    let htmlContent = '';
    results.forEach(result => {
        // Skip special messages that might be at the end
        if (result.lineNumber === -1) {
            // Ensure special content is escaped
            htmlContent += `<div class="info-message">${escapeHtml(result.content)}</div>`;
            return;
        }

        // Escape line number for safety (although it should be a number)
        const safeLineNumber = typeof result.lineNumber === 'number' ?
            result.lineNumber : escapeHtml(String(result.lineNumber));

        // Highlight search terms in content (ensuring it's escaped)
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
    // Validate parameters
    if (!text || typeof text !== 'string') {
        console.error('Error: invalid text in highlightSearchTerm');
        return '';
    }

    if (!term || typeof term !== 'string') {
        console.error('Error: invalid search term in highlightSearchTerm');
        return escapeHtml(text);
    }

    // First escape the text to prevent XSS
    const safeText = escapeHtml(text);

    // Escape special regex characters
    const escapedTerm = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

    try {
        const regex = new RegExp(`(${escapedTerm})`, 'gi');
        return safeText.replace(regex, (match) => `<span class="result-match">${match}</span>`);
    } catch (error) {
        console.error('Error highlighting text:', error);
        return safeText; // On error, return escaped text
    }
}

// ── File Viewer ──────────────────────────────────────────────────────────────

function openFileViewer(filePath, fileName) {
    if (!filePath || typeof filePath !== 'string') return;

    var safeFileName = escapeHtml(fileName || 'File');
    document.getElementById('fileViewerFileName').textContent = safeFileName;
    document.getElementById('fileViewerContent').textContent = 'Loading...';
    document.getElementById('fileViewerModal').style.display = 'flex';
    document.body.style.overflow = 'hidden';

    fetch('/file-content?path=' + encodeURIComponent(filePath))
        .then(function (response) {
            if (!response.ok) {
                return response.text().then(function (text) {
                    throw new Error(text.trim() || 'Cannot load file');
                });
            }
            return response.json();
        })
        .then(function (data) {
            document.getElementById('fileViewerContent').textContent = data.content;
        })
        .catch(function (error) {
            document.getElementById('fileViewerContent').textContent = 'Error: ' + error.message;
        });
}

function closeFileViewer() {
    document.getElementById('fileViewerModal').style.display = 'none';
    document.body.style.overflow = 'auto';
    document.getElementById('fileViewerContent').textContent = '';
}

// Kept for backward compatibility — openFileViewer is the active implementation
function fetchFileContent(filePath) {
    openFileViewer(filePath, '');
}

// ── File Selection (cross-directory via sessionStorage) ───────────────────────

var SELECTION_KEY = 'upgopher_selected_files';

function getSelectedFiles() {
    try {
        return JSON.parse(sessionStorage.getItem(SELECTION_KEY) || '[]');
    } catch (e) {
        return [];
    }
}

function setSelectedFiles(paths) {
    sessionStorage.setItem(SELECTION_KEY, JSON.stringify(paths));
    updateDownloadSelectedBtn();
}

function initCheckboxes() {
    var selected = getSelectedFiles();
    document.querySelectorAll('.file-select-checkbox').forEach(function (cb) {
        cb.checked = selected.indexOf(cb.dataset.path) !== -1;
    });
    updateDownloadSelectedBtn();
    updateSelectAllCheckbox();
}

function onCheckboxChange(checkbox) {
    var path = checkbox.dataset.path;
    var selected = getSelectedFiles();
    if (checkbox.checked) {
        if (selected.indexOf(path) === -1) selected.push(path);
    } else {
        selected = selected.filter(function (p) { return p !== path; });
    }
    setSelectedFiles(selected);
    updateSelectAllCheckbox();
}

function selectAll(checked) {
    var checkboxes = document.querySelectorAll('.file-select-checkbox');
    var selected = getSelectedFiles();
    if (checked) {
        checkboxes.forEach(function (cb) {
            cb.checked = true;
            if (selected.indexOf(cb.dataset.path) === -1) {
                selected.push(cb.dataset.path);
            }
        });
    } else {
        var pagePaths = Array.from(checkboxes).map(function (cb) { return cb.dataset.path; });
        selected = selected.filter(function (p) { return pagePaths.indexOf(p) === -1; });
        checkboxes.forEach(function (cb) { cb.checked = false; });
    }
    setSelectedFiles(selected);
}

function updateSelectAllCheckbox() {
    var checkboxes = Array.from(document.querySelectorAll('.file-select-checkbox'));
    var selectAllCb = document.getElementById('selectAllCheckbox');
    if (!selectAllCb || checkboxes.length === 0) return;
    var allChecked = checkboxes.every(function (cb) { return cb.checked; });
    var someChecked = checkboxes.some(function (cb) { return cb.checked; });
    selectAllCb.checked = allChecked;
    selectAllCb.indeterminate = !allChecked && someChecked;
}

function updateDownloadSelectedBtn() {
    var selected = getSelectedFiles();
    var btn = document.getElementById('downloadSelectedBtn');
    if (!btn) return;
    if (selected.length === 0) {
        btn.style.display = 'none';
    } else {
        btn.style.display = 'inline-block';
        btn.innerHTML = '<i class="fa fa-download"></i> Download Selected (' + selected.length + ')';
    }
}

function downloadSelected() {
    var selected = getSelectedFiles();
    if (selected.length === 0) {
        showToast('No items selected', 'error');
        return;
    }

    fetch('/zip-selected', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths: selected })
    })
        .then(function (response) {
            if (!response.ok) {
                return response.text().then(function (text) {
                    throw new Error(text.trim() || 'Error creating ZIP');
                });
            }
            return response.blob();
        })
        .then(function (blob) {
            var url = URL.createObjectURL(blob);
            var a = document.createElement('a');
            a.href = url;
            a.download = 'selected-files.zip';
            document.body.appendChild(a);
            a.click();
            setTimeout(function () {
                URL.revokeObjectURL(url);
                document.body.removeChild(a);
            }, 100);
        })
        .catch(function (error) {
            showToast('Error downloading: ' + escapeHtml(error.message), 'error');
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

        // Last Modified column (date sorting) — now at index 4 due to checkbox column
        if (n === 4) {
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
    const iconId = n === 1 ? 'name-icon' :
        n === 3 ? 'size-icon' :
            n === 4 ? 'modified-icon' :
                n === 5 ? 'custom-path-icon' : '';
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
    var uploadForm = document.getElementById('upload-form');
    var fileInput = document.getElementById('file-upload');
    var folderInput = document.getElementById('folder-upload');
    var fileNameDisplay = document.getElementById('file-name');

    if (!uploadForm || !fileInput) {
        console.error('Upload form or file input not found');
        return;
    }

    // Show selected file count when file picker changes
    fileInput.addEventListener('change', function() {
        if (fileNameDisplay) {
            var count = fileInput.files.length;
            if (count === 0) {
                fileNameDisplay.textContent = '';
            } else if (count === 1) {
                fileNameDisplay.textContent = fileInput.files[0].name;
            } else {
                fileNameDisplay.textContent = count + ' files selected';
            }
        }
    });

    if (folderInput) {
        folderInput.addEventListener('change', function() {
            if (fileNameDisplay && folderInput.files.length > 0) {
                var firstPath = folderInput.files[0].webkitRelativePath;
                var folderName = firstPath.split('/')[0];
                fileNameDisplay.textContent = folderName + ' (' + folderInput.files.length + ' files)';
            }
        });
    }

    uploadForm.addEventListener('submit', function(e) {
        e.preventDefault();

        var fileItems = [];

        // Check file input (multi-file or single)
        if (fileInput.files && fileInput.files.length > 0) {
            for (var i = 0; i < fileInput.files.length; i++) {
                var f = fileInput.files[i];
                fileItems.push({
                    file: f,
                    relativePath: f.webkitRelativePath || f.name
                });
            }
        }

        // Check folder input (if no files were chosen from file input)
        if (fileItems.length === 0 && folderInput && folderInput.files && folderInput.files.length > 0) {
            for (var j = 0; j < folderInput.files.length; j++) {
                var ff = folderInput.files[j];
                fileItems.push({
                    file: ff,
                    relativePath: ff.webkitRelativePath || ff.name
                });
            }
        }

        if (fileItems.length === 0) {
            alert('Por favor selecciona un archivo o carpeta');
            return;
        }

        uploadFiles(fileItems, []);
    });
}

// ── Clipboard SSE Auto-Sync ───────────────────────────────────────────────────
var _sseSource = null;           // active EventSource
var _autoSaveTimer = null;       // debounce timer handle
var _autoSaveEnabled = true;     // current state
var _isLocallyDirty = false;     // true when user has typed but the save hasn't completed yet
var AUTOSAVE_DEBOUNCE_MS = 1200; // ms to wait after last keystroke before saving

/**
 * Reads the auto-save preference from localStorage and applies it.
 * Called once on load. Default is enabled.
 */
function initAutoSave() {
    var stored = localStorage.getItem('upgopher_autosave');
    _autoSaveEnabled = stored !== '0'; // default ON
    var cb = document.getElementById('clipboard-autosave-toggle');
    if (cb) cb.checked = _autoSaveEnabled;
    var textarea = document.getElementById('shared-clipboard-textarea');
    if (textarea) {
        textarea.addEventListener('input', onClipboardInput);
    }
}

/** Called on every textarea input event. */
function onClipboardInput() {
    if (!_autoSaveEnabled) return;
    _isLocallyDirty = true;        // mark that the textarea has unsaved local changes
    clearTimeout(_autoSaveTimer);
    setAutoSaveStatus('pending');
    _autoSaveTimer = setTimeout(function () {
        saveToSharedClipboard(true); // true = called by auto-save
    }, AUTOSAVE_DEBOUNCE_MS);
}

/** Toggle auto-save ON/OFF from the UI checkbox. */
function toggleAutoSave(enabled) {
    _autoSaveEnabled = enabled;
    localStorage.setItem('upgopher_autosave', enabled ? '1' : '0');
    if (!enabled) {
        clearTimeout(_autoSaveTimer);
        setAutoSaveStatus('off');
    } else {
        setAutoSaveStatus('idle');
    }
}

/**
 * Updates the auto-save status indicator.
 * states: 'idle' | 'pending' | 'saving' | 'saved' | 'off'
 */
function setAutoSaveStatus(state) {
    var el = document.getElementById('autosave-status');
    if (!el) return;
    var labels = { idle: '', pending: '…', saving: 'Saving…', saved: 'Auto-saved ✓', off: 'Auto-save off' };
    el.textContent = labels[state] || '';
    el.className = 'autosave-status autosave-' + state;
}

// ── Server-Sent Events connection ─────────────────────────────────────────────

/**
 * Opens an SSE connection to /clipboard/stream?tab=<tabName>.
 * Closes any existing connection first.
 * When a "change" event arrives from the server (because another client saved),
 * we re-fetch the tab content — but only if the user is not currently editing.
 */
function connectClipboardSSE(tabName) {
    // Close previous connection
    if (_sseSource) {
        _sseSource.close();
        _sseSource = null;
    }

    var url = '/clipboard/stream?tab=' + encodeURIComponent(tabName);
    // For protected tabs, we cannot pass the token via EventSource URL cleanly.
    // We rely on the tab already being accessible (token validated on GET /clipboard).
    // If the tab is protected and we don't have the token yet, skip SSE — the
    // token-unlock flow will call selectClipboardTab which re-calls us.
    var token = clipboardTokenCache[tabName];
    if (token) {
        url += '&X-Tab-Token=' + encodeURIComponent(token);
    }

    var es = new EventSource(url);
    _sseSource = es;

    es.addEventListener('change', function (e) {
        var changedTab = e.data.trim();
        // Only react if we're still on the same tab
        if (changedTab !== currentClipboardTab) return;
        // Don't overwrite while the user has unsaved local edits.
        // We use a dirty flag rather than checking activeElement so that
        // users who only have focus (e.g. to read/copy) still receive updates.
        if (_isLocallyDirty) return;
        // Re-fetch content silently
        fetchClipboardContent(changedTab);
    });

    es.addEventListener('error', function () {
        // EventSource handles reconnection automatically.
        // If permanently unavailable (e.g. tab deleted), the next selectClipboardTab will clean up.
    });
}

/**
 * Fetches the content of tabName and updates the textarea + meta bar.
 * Does NOT change currentClipboardTab — used for silent background refreshes.
 */
function fetchClipboardContent(tabName) {
    var headers = {};
    var token = clipboardTokenCache[tabName];
    if (token) headers['X-Tab-Token'] = token;

    fetch('/clipboard?tab=' + encodeURIComponent(tabName), { headers: headers })
        .then(function (r) {
            if (!r.ok) return null;
            return r.text();
        })
        .then(function (content) {
            if (content === null || content === undefined) return;
            document.getElementById('shared-clipboard-textarea').value = content;
            document.getElementById('clipboard-char-count').textContent = 'chars: ' + content.length;
            // Refresh tab metadata to update updatedAt
            loadClipboardTabs(false);
        })
        .catch(function () { /* silent */ });
}

// ── Clipboard multi-tab management ───────────────────────────────────────────
var currentClipboardTab = 'default';
var clipboardTabsCache = [];
var clipboardTokenCache = {}; // tabName → plaintext token (in-memory only)
var _tabSelectionSeq  = 0;   // incremented on each selectClipboardTab call to discard stale responses
var _tokenUnlockTabName = null; // tab name for which the unlock row is currently shown

function loadClipboardTabs(selectCurrent) {
    if (selectCurrent === undefined) selectCurrent = true;

    fetch('/clipboard/tabs')
        .then(function (r) {
            if (!r.ok) throw new Error('Failed to load tabs');
            return r.json();
        })
        .then(function (tabs) {
            clipboardTabsCache = tabs;
            renderClipboardTabs(tabs);
            if (selectCurrent) {
                var exists = tabs.some(function (t) { return t.name === currentClipboardTab; });
                selectClipboardTab(exists ? currentClipboardTab : 'default');
            } else {
                var entry = tabs.find(function (t) { return t.name === currentClipboardTab; });
                if (entry) updateClipboardMeta(entry);
                updateForgetTokenBtn();
            }
        })
        .catch(function (err) { console.error('Error loading clipboard tabs:', err); });
}

function renderClipboardTabs(tabs) {
    var list = document.getElementById('clipboard-tabs-list');
    list.innerHTML = '';

    // "default" always first, then alphabetical
    tabs.sort(function (a, b) {
        if (a.name === 'default') return -1;
        if (b.name === 'default') return 1;
        return a.name.localeCompare(b.name);
    });

    tabs.forEach(function (tab) {
        var item = document.createElement('div');
        item.className = 'clipboard-tab-item' + (tab.name === currentClipboardTab ? ' active' : '');
        item.dataset.tabName = tab.name;

        var label = document.createElement('button');
        label.className = 'clipboard-tab-label';
        if (tab.protected) {
            var lock = document.createElement('i');
            lock.className = 'fa fa-lock tab-lock-icon';
            label.appendChild(lock);
            label.appendChild(document.createTextNode(tab.name));
        } else {
            label.textContent = tab.name;
        }
        label.onclick = (function (name) {
            return function () { selectClipboardTab(name); };
        })(tab.name);
        item.appendChild(label);

        if (tab.name !== 'default') {
            var closeBtn = document.createElement('button');
            closeBtn.className = 'clipboard-tab-close';
            closeBtn.innerHTML = '&times;';
            closeBtn.title = 'Delete tab';
            closeBtn.onclick = (function (el, name) {
                return function (e) { e.stopPropagation(); showDeleteConfirm(el, name); };
            })(item, tab.name);
            item.appendChild(closeBtn);
        }

        list.appendChild(item);
    });
}

function selectClipboardTab(name) {
    currentClipboardTab = name;
    _isLocallyDirty = false; // switching tabs resets any local unsaved state
    var seq = ++_tabSelectionSeq; // capture sequence number — stale responses are discarded

    document.querySelectorAll('#clipboard-tabs-list .clipboard-tab-item').forEach(function (el) {
        el.classList.toggle('active', el.dataset.tabName === name);
    });

    var headers = {};
    var cachedToken = clipboardTokenCache[name];
    if (cachedToken) headers['X-Tab-Token'] = cachedToken;

    fetch('/clipboard?tab=' + encodeURIComponent(name), { headers: headers })
        .then(function (r) {
            if (seq !== _tabSelectionSeq) return null; // stale response — a newer call supersedes this one
            if (r.status === 401) {
                document.getElementById('shared-clipboard-textarea').value = '';
                document.getElementById('clipboard-char-count').textContent = 'chars: 0';
                setClipboardEditable(false);
                showTokenUnlockRow(name, function () { selectClipboardTab(name); });
                return null;
            }
            if (!r.ok) throw new Error('Tab not found');
            return r.text();
        })
        .then(function (content) {
            if (content === null || content === undefined) return;
            if (seq !== _tabSelectionSeq) return; // stale response
            hideTokenUnlockRow();
            setClipboardEditable(true);
            document.getElementById('shared-clipboard-textarea').value = content;
            document.getElementById('clipboard-char-count').textContent = 'chars: ' + content.length;
            var entry = clipboardTabsCache.find(function (t) { return t.name === name; });
            if (entry) updateClipboardMeta(entry);
            updateForgetTokenBtn();
            // Connect SSE stream for real-time updates from other clients
            connectClipboardSSE(name);
        })
        .catch(function (err) { if (seq === _tabSelectionSeq) console.error('Error loading tab:', err); });
}

function updateClipboardMeta(tab) {
    var updEl = document.getElementById('clipboard-updated');
    if (!tab || !tab.updatedAt) { updEl.textContent = 'updated: --'; return; }
    var d = new Date(tab.updatedAt);
    updEl.textContent = 'updated: ' + d.toLocaleTimeString();
}

function showNewTabInput() {
    var row = document.getElementById('new-tab-input-row');
    row.style.display = 'flex';
    document.getElementById('new-tab-name').value = '';
    document.getElementById('new-tab-protect').checked = false;
    document.getElementById('new-tab-custom-token').value = '';
    document.getElementById('new-tab-custom-token').style.display = 'none';
    document.getElementById('new-tab-name').focus();
}

function hideNewTabInput() {
    document.getElementById('new-tab-input-row').style.display = 'none';
    document.getElementById('new-tab-protect').checked = false;
    document.getElementById('new-tab-custom-token').value = '';
    document.getElementById('new-tab-custom-token').style.display = 'none';
}

/** Shows or hides the optional custom-token password input. */
function toggleCustomTokenInput(checked) {
    var tokenInput = document.getElementById('new-tab-custom-token');
    tokenInput.style.display = checked ? 'block' : 'none';
    if (!checked) tokenInput.value = '';
}

function createClipboardTab() {
    var nameInput = document.getElementById('new-tab-name');
    var name = nameInput.value.trim();
    var protect = document.getElementById('new-tab-protect').checked;
    var customToken = protect ? document.getElementById('new-tab-custom-token').value : '';

    if (!name) { showToast('Tab name cannot be empty', 'error'); return; }
    if (!/^[a-zA-Z0-9 _-]{1,50}$/.test(name)) {
        showToast('Invalid name (only letters, numbers, spaces, - and _)', 'error');
        return;
    }
    if (protect && customToken !== '' && customToken.length < 6) {
        showToast('Custom password must be at least 6 characters', 'error');
        return;
    }

    var headers = { 'Content-Type': 'text/plain' };
    if (protect) {
        headers['X-Tab-Token-Create'] = '1';
        if (customToken) headers['X-Tab-Token-Value'] = customToken;
    }

    fetch('/clipboard?tab=' + encodeURIComponent(name), {
        method: 'POST',
        headers: headers,
        body: ''
    })
        .then(function (r) {
            if (!r.ok) return r.text().then(function (t) { throw new Error(t.trim()); });
            var generatedToken = r.headers.get('X-Generated-Token');
            hideNewTabInput();
            currentClipboardTab = name;
            if (generatedToken) {
                // Auto-generated: cache and reveal in modal
                clipboardTokenCache[name] = generatedToken;
            } else if (protect && customToken) {
                // Custom token: cache it immediately — user typed it themselves
                clipboardTokenCache[name] = customToken;
            }
            document.getElementById('shared-clipboard-textarea').value = '';
            document.getElementById('clipboard-char-count').textContent = 'chars: 0';
            setClipboardEditable(true);
            loadClipboardTabs(false);
            // Connect SSE for the newly created tab immediately.
            connectClipboardSSE(name);
            if (generatedToken) {
                showTokenRevealModal(generatedToken, name);
            } else if (protect && customToken) {
                showToast('Tab "' + name + '" created with your password');
            } else {
                showToast('Tab "' + name + '" created');
            }
        })
        .catch(function (err) { showToast(err.message, 'error'); });
}

function showDeleteConfirm(tabElement, tabName) {
    if (tabElement.querySelector('.clipboard-tab-confirm')) {
        cancelDeleteConfirm(tabElement);
        return;
    }
    var closeBtn = tabElement.querySelector('.clipboard-tab-close');
    if (closeBtn) closeBtn.style.display = 'none';

    var confirm = document.createElement('span');
    confirm.className = 'clipboard-tab-confirm';

    var yes = document.createElement('button');
    yes.className = 'clipboard-tab-confirm-yes';
    yes.textContent = '\u2713';
    yes.title = 'Confirm delete';
    yes.onclick = (function (name) {
        return function (e) { e.stopPropagation(); deleteClipboardTab(name); };
    })(tabName);

    var no = document.createElement('button');
    no.className = 'clipboard-tab-confirm-no';
    no.textContent = '\u2717';
    no.title = 'Cancel';
    no.onclick = (function (el) {
        return function (e) { e.stopPropagation(); cancelDeleteConfirm(el); };
    })(tabElement);

    confirm.appendChild(yes);
    confirm.appendChild(no);
    tabElement.appendChild(confirm);
}

function cancelDeleteConfirm(tabElement) {
    var confirm = tabElement.querySelector('.clipboard-tab-confirm');
    if (confirm) confirm.remove();
    var closeBtn = tabElement.querySelector('.clipboard-tab-close');
    if (closeBtn) closeBtn.style.display = '';
}

function deleteClipboardTab(name) {
    var headers = {};
    var token = clipboardTokenCache[name];
    if (token) headers['X-Tab-Token'] = token;

    fetch('/clipboard?tab=' + encodeURIComponent(name), { method: 'DELETE', headers: headers })
        .then(function (r) {
            if (r.status === 401) {
                showTokenUnlockRow(name, function () { deleteClipboardTab(name); });
                return;
            }
            if (!r.ok) return r.text().then(function (t) { throw new Error(t.trim()); });
            showToast('Tab "' + name + '" deleted');
            delete clipboardTokenCache[name];
            if (currentClipboardTab === name) currentClipboardTab = 'default';
            loadClipboardTabs(false);
            selectClipboardTab(currentClipboardTab);
        })
        .catch(function (err) { showToast(err.message, 'error'); });
}

// ── Token management ──────────────────────────────────────────────────────────
var _tokenUnlockCallback = null;

function setClipboardEditable(editable) {
    var ta = document.getElementById('shared-clipboard-textarea');
    if (!ta) return;
    ta.disabled = !editable;
    if (editable) {
        ta.classList.remove('clipboard-locked');
        ta.placeholder = 'Paste your text here to share it...';
    } else {
        ta.classList.add('clipboard-locked');
        ta.placeholder = 'Enter the token above to be able to write here...';
    }
}

function refreshClipboardTab() {
    var btn = document.getElementById('refresh-clipboard-btn');
    var icon = btn ? btn.querySelector('i') : null;
    if (icon) icon.classList.add('spinning');
    var prevSeq = _tabSelectionSeq;
    selectClipboardTab(currentClipboardTab);
    // Remove spinning class after a short delay to cover the fetch duration
    setTimeout(function () {
        if (icon) icon.classList.remove('spinning');
    }, 600);
}

function showTokenUnlockRow(tabName, callback) {
    _tokenUnlockCallback = callback;
    _tokenUnlockTabName  = tabName; // remember which tab triggered this row
    var row = document.getElementById('token-unlock-row');
    row.style.display = 'flex';
    var input = document.getElementById('token-unlock-input');
    input.value = '';
    input.focus();
}

function hideTokenUnlockRow() {
    document.getElementById('token-unlock-row').style.display = 'none';
    _tokenUnlockCallback = null;
    _tokenUnlockTabName  = null;
}

function submitTabToken() {
    var token    = document.getElementById('token-unlock-input').value.trim();
    var tabName  = _tokenUnlockTabName || currentClipboardTab;
    var callback = _tokenUnlockCallback; // capture before hideTokenUnlockRow nulls it
    if (!token) { showToast('Please enter the token', 'error'); return; }
    clipboardTokenCache[tabName] = token;
    currentClipboardTab = tabName;
    hideTokenUnlockRow();
    updateForgetTokenBtn();
    if (callback) callback();
}

function forgetTabToken() {
    delete clipboardTokenCache[currentClipboardTab];
    document.getElementById('shared-clipboard-textarea').value = '';
    document.getElementById('clipboard-char-count').textContent = 'chars: 0';
    setClipboardEditable(false);
    var entry = clipboardTabsCache.find(function (t) { return t.name === currentClipboardTab; });
    if (entry && entry.protected) {
        showTokenUnlockRow(currentClipboardTab, function () { selectClipboardTab(currentClipboardTab); });
    }
    updateForgetTokenBtn();
}

function updateForgetTokenBtn() {
    var btn = document.getElementById('forget-token-btn');
    if (!btn) return;
    var entry = clipboardTabsCache.find(function (t) { return t.name === currentClipboardTab; });
    btn.style.display = (entry && entry.protected && clipboardTokenCache[currentClipboardTab]) ? 'inline-flex' : 'none';
}

function showTokenRevealModal(token) {
    document.getElementById('token-reveal-value').textContent = token;
    document.getElementById('tokenRevealModal').style.display = 'flex';
}

function closeTokenRevealModal() {
    document.getElementById('tokenRevealModal').style.display = 'none';
    showToast('Tab "' + currentClipboardTab + '" created (protected)');
}

function copyGeneratedToken() {
    var token = document.getElementById('token-reveal-value').textContent;
    navigator.clipboard.writeText(token)
        .then(function () { showToast('Token copied to clipboard'); })
        .catch(function () {
            var ta = document.createElement('textarea');
            ta.value = token;
            ta.style.position = 'fixed';
            ta.style.left = '-999999px';
            document.body.appendChild(ta);
            ta.focus();
            ta.select();
            document.execCommand('copy');
            document.body.removeChild(ta);
            showToast('Token copied to clipboard');
        });
}

// ── Toast notifications ───────────────────────────────────────────────────────
function showToast(message, type) {
    if (!type) type = 'success';
    var container = document.getElementById('toast-container');
    if (!container) return;
    var toast = document.createElement('div');
    toast.className = 'toast ' + type;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(function () {
        toast.classList.add('hiding');
        toast.addEventListener('animationend', function () { toast.remove(); });
    }, 3000);
}

// ── New Folder modal ──────────────────────────────────────────────────────────
function showNewFolderModal() {
    document.getElementById('newFolderName').value = '';
    document.getElementById('newFolderError').style.display = 'none';
    document.getElementById('newFolderError').textContent = '';
    document.getElementById('newFolderModal').style.display = 'flex';
    document.body.style.overflow = 'hidden';
    setTimeout(function () { document.getElementById('newFolderName').focus(); }, 100);
}

function closeNewFolderModal() {
    document.getElementById('newFolderModal').style.display = 'none';
    document.body.style.overflow = 'auto';
}

function createFolder() {
    var folderName = document.getElementById('newFolderName').value.trim();
    var errorEl = document.getElementById('newFolderError');

    if (!folderName) {
        errorEl.textContent = 'Please enter a folder name.';
        errorEl.style.display = 'block';
        return;
    }

    if (!/^[a-zA-Z0-9_-]+$/.test(folderName)) {
        errorEl.textContent = 'Invalid name: only letters, digits, hyphens and underscores are allowed.';
        errorEl.style.display = 'block';
        return;
    }

    var params = new URLSearchParams(window.location.search);
    var currentPath = params.get('path') || '';

    fetch('/mkdir', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'folderName=' + encodeURIComponent(folderName) + '&currentPath=' + encodeURIComponent(currentPath)
    })
        .then(function (response) {
            if (response.status === 409) {
                errorEl.textContent = 'A folder with that name already exists.';
                errorEl.style.display = 'block';
                return;
            }
            if (!response.ok) {
                return response.text().then(function (text) {
                    errorEl.textContent = text.trim() || 'Failed to create folder.';
                    errorEl.style.display = 'block';
                });
            }
            closeNewFolderModal();
            window.location.reload();
        })
        .catch(function (error) {
            errorEl.textContent = 'Network error: ' + escapeHtml(error.message);
            errorEl.style.display = 'block';
        });
}

// ── Error modal (directory operations) ───────────────────────────────────────
function showErrorModal(message) {
    document.getElementById('errorModalMessage').textContent = message;
    document.getElementById('errorModal').style.display = 'flex';
    document.body.style.overflow = 'hidden';
}

function closeErrorModal() {
    document.getElementById('errorModal').style.display = 'none';
    document.body.style.overflow = 'auto';
}

function deleteFolder(encodedPath) {
    fetch('/delete/?path=' + encodeURIComponent(encodedPath))
        .then(function (response) {
            if (response.ok || response.redirected) {
                window.location.reload();
                return;
            }
            if (response.status === 403) {
                return response.text().then(function (text) {
                    showErrorModal(text.trim() || 'Cannot delete directory.');
                });
            }
            return response.text().then(function (text) {
                showErrorModal(text.trim() || 'An error occurred while deleting the folder.');
            });
        })
        .catch(function (error) {
            showErrorModal('Network error: ' + escapeHtml(error.message));
        });
}

// ── Directory Tree Panel ──────────────────────────────────────────────────────

var _treeLoaded = false; // whether the root tree has been fetched at least once

/**
 * Toggle the tree sidebar open/closed.
 * On first open the root tree is fetched from /api/v1/tree.
 */
function toggleTreePanel() {
    var overlay = document.getElementById('treePanelOverlay');
    if (!overlay) return;

    if (overlay.style.display === 'none' || overlay.style.display === '') {
        overlay.style.display = 'block';
        document.body.style.overflow = 'hidden';
        if (!_treeLoaded) {
            loadTreeRoot();
        }
    } else {
        closeTreePanel();
    }
}

/** Close and hide the tree panel. */
function closeTreePanel() {
    var overlay = document.getElementById('treePanelOverlay');
    if (!overlay) return;
    overlay.style.display = 'none';
    document.body.style.overflow = 'auto';
}

/**
 * Fetch the root tree (depth=1) and render it into #treeContainer.
 * Subsequent lazy-loads happen per-node inside renderTreeNode.
 */
function loadTreeRoot() {
    var container = document.getElementById('treeContainer');
    if (!container) return;

    container.innerHTML = '<div class="tree-loading">Loading directory tree…</div>';

    fetch('/api/v1/tree?depth=1')
        .then(function (r) {
            if (!r.ok) throw new Error('Failed to load tree');
            return r.json();
        })
        .then(function (root) {
            _treeLoaded = true;
            container.innerHTML = '';

            if (!root.children || root.children.length === 0) {
                container.innerHTML = '<div class="tree-empty">No folders found.</div>';
                return;
            }

            // Render root's children directly (the shared root itself is implicit)
            var rootEl = document.createElement('div');
            rootEl.className = 'tree-root';
            renderTreeNode(root, rootEl, true);
            container.appendChild(rootEl);
        })
        .catch(function (err) {
            container.innerHTML = '<div class="tree-loading">Error loading tree: ' + escapeHtml(err.message) + '</div>';
        });
}

/**
 * Read the current page path from the hidden input so we can highlight
 * the active folder in the tree.
 */
function getCurrentTreePath() {
    var pathInput = document.getElementById('current-path-value');
    return pathInput ? pathInput.value : '';
}

/**
 * Build the DOM for one tree node and append it to parentEl.
 *
 * @param {object}  node       - JSON node from /api/v1/tree
 * @param {Element} parentEl   - DOM element to append into
 * @param {boolean} isRoot     - true when rendering the shared-root node
 */
function renderTreeNode(node, parentEl, isRoot) {
    var currentPath = getCurrentTreePath();
    var isActive = (node.path !== '' && node.path === currentPath) ||
                   (node.path === '' && currentPath === '');

    var nodeEl = document.createElement('div');
    nodeEl.className = 'tree-node';
    if (isRoot) nodeEl.className += ' tree-root';

    // --- row ---
    var rowEl = document.createElement('div');
    rowEl.className = 'tree-node-row' + (isActive ? ' tree-active' : '');

    // Toggle button (only if node has children)
    var toggleEl;
    if (node.hasChildren || (node.children && node.children.length > 0)) {
        toggleEl = document.createElement('button');
        toggleEl.className = 'tree-toggle';
        toggleEl.innerHTML = '&#9658;'; // ▶
        toggleEl.title = 'Expand / Collapse';
    } else {
        var spacer = document.createElement('span');
        spacer.className = 'tree-toggle-spacer';
        rowEl.appendChild(spacer);
    }

    // Folder icon
    var iconEl = document.createElement('i');
    iconEl.className = 'fa fa-folder tree-icon';

    // Label (click = navigate)
    var labelEl = document.createElement('span');
    labelEl.className = 'tree-label';
    labelEl.textContent = node.name;
    labelEl.title = node.name;
    if (node.path !== undefined) {
        (function (encodedPath) {
            labelEl.addEventListener('click', function () {
                navigateToFolder(encodedPath);
            });
        })(node.path);
    }

    // Children container (lazy)
    var childrenEl = document.createElement('div');
    childrenEl.className = 'tree-children';
    var childrenPopulated = false;

    // Populate pre-loaded children
    if (node.children && node.children.length > 0) {
        node.children.forEach(function (child) {
            renderTreeNode(child, childrenEl, false);
        });
        childrenPopulated = true;
    }

    // Wire up toggle
    if (toggleEl) {
        (function (te, ce, nodePath, populated) {
            te.addEventListener('click', function () {
                var isOpen = ce.classList.contains('expanded');
                if (isOpen) {
                    ce.classList.remove('expanded');
                    te.classList.remove('open');
                    te.innerHTML = '&#9658;';
                    iconEl.className = 'fa fa-folder tree-icon';
                } else {
                    // Lazy-load children if not yet done
                    if (!populated && nodePath !== '') {
                        ce.innerHTML = '<div class="tree-loading-inline"><i class="fa fa-spinner"></i> Loading…</div>';
                        loadTreeChildren(nodePath, ce, function () {
                            childrenPopulated = true;
                            populated = true;
                        });
                    }
                    ce.classList.add('expanded');
                    te.classList.add('open');
                    te.innerHTML = '&#9660;';
                    iconEl.className = 'fa fa-folder-open tree-icon';
                }
            });
        })(toggleEl, childrenEl, node.path, childrenPopulated);

        rowEl.appendChild(toggleEl);
    }

    rowEl.appendChild(iconEl);
    rowEl.appendChild(labelEl);
    nodeEl.appendChild(rowEl);
    nodeEl.appendChild(childrenEl);
    parentEl.appendChild(nodeEl);

    // Auto-expand if current path starts with this node's path
    if (isActive && toggleEl) {
        toggleEl.click();
    }
}

/**
 * Fetch the immediate children of a folder and render them into containerEl.
 *
 * @param {string}   encodedPath  - base64-encoded relative path
 * @param {Element}  containerEl  - .tree-children element to populate
 * @param {Function} onDone       - called after rendering
 */
function loadTreeChildren(encodedPath, containerEl, onDone) {
    fetch('/api/v1/tree?path=' + encodeURIComponent(encodedPath) + '&depth=1')
        .then(function (r) {
            if (!r.ok) throw new Error('fetch failed');
            return r.json();
        })
        .then(function (node) {
            containerEl.innerHTML = '';
            if (!node.children || node.children.length === 0) {
                containerEl.innerHTML = '<div class="tree-empty" style="padding:4px 12px;font-size:12px;">Empty</div>';
            } else {
                node.children.forEach(function (child) {
                    renderTreeNode(child, containerEl, false);
                });
            }
            if (onDone) onDone();
        })
        .catch(function (err) {
            containerEl.innerHTML = '<div class="tree-loading-inline">Error: ' + escapeHtml(err.message) + '</div>';
        });
}

/**
 * Expand every node in the tree by fetching the full tree with depth=-1.
 * This replaces the lazily-built DOM with a complete tree.
 */
function expandAllTree() {
    var container = document.getElementById('treeContainer');
    if (!container) return;

    container.innerHTML = '<div class="tree-loading">Loading full tree…</div>';

    fetch('/api/v1/tree?depth=10') // cap at 10 levels for performance
        .then(function (r) {
            if (!r.ok) throw new Error('Failed to load tree');
            return r.json();
        })
        .then(function (root) {
            container.innerHTML = '';
            if (!root.children || root.children.length === 0) {
                container.innerHTML = '<div class="tree-empty">No folders found.</div>';
                return;
            }
            var rootEl = document.createElement('div');
            rootEl.className = 'tree-root';
            renderTreeNode(root, rootEl, true);
            container.appendChild(rootEl);

            // Expand all toggle buttons in the rendered tree
            container.querySelectorAll('.tree-toggle').forEach(function (btn) {
                if (!btn.classList.contains('open')) {
                    btn.click();
                }
            });
        })
        .catch(function (err) {
            container.innerHTML = '<div class="tree-loading">Error: ' + escapeHtml(err.message) + '</div>';
        });
}

/** Collapse all expanded nodes back to root level. */
function collapseAllTree() {
    var container = document.getElementById('treeContainer');
    if (!container) return;

    container.querySelectorAll('.tree-children.expanded').forEach(function (el) {
        el.classList.remove('expanded');
    });
    container.querySelectorAll('.tree-toggle.open').forEach(function (btn) {
        btn.classList.remove('open');
        btn.innerHTML = '&#9658;';
    });
    container.querySelectorAll('.fa-folder-open').forEach(function (icon) {
        icon.className = 'fa fa-folder tree-icon';
    });
}

/**
 * Navigate the main file listing to the given folder.
 *
 * @param {string} encodedPath - base64-encoded relative path (empty = root)
 */
function navigateToFolder(encodedPath) {
    if (encodedPath === '' || encodedPath === null || encodedPath === undefined) {
        window.location.href = '/';
    } else {
        window.location.href = '/?path=' + encodeURIComponent(encodedPath);
    }
}
