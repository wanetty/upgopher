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
function saveToSharedClipboard() {
    var clipboardText = document.getElementById('shared-clipboard-textarea').value;
    var headers = { 'Content-Type': 'text/plain' };
    var token = clipboardTokenCache[currentClipboardTab];
    if (token) headers['X-Tab-Token'] = token;

    fetch('/clipboard?tab=' + encodeURIComponent(currentClipboardTab), {
        method: 'POST',
        headers: headers,
        body: clipboardText
    })
        .then(function (response) {
            if (response.status === 401) {
                setClipboardEditable(false);
                showTokenUnlockRow(currentClipboardTab, function () { saveToSharedClipboard(); });
                return;
            }
            if (!response.ok) throw new Error('Error saving to shared clipboard');
            showToast('Saved to "' + currentClipboardTab + '"');
            loadClipboardTabs(false);
        })
        .catch(function (error) {
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
    xhr.upload.addEventListener('progress', function (e) {
        if (e.lengthComputable) {
            const percentage = (e.loaded / e.total) * 100;
            updateUploadProgress(e.loaded, e.total, percentage);
        }
    });

    // Upload complete event
    xhr.addEventListener('load', function () {
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
    xhr.addEventListener('error', function () {
        hideUploadProgress();
        alert('Error al subir el archivo');
    });

    // Upload abort event
    xhr.addEventListener('abort', function () {
        hideUploadProgress();
        alert('Subida de archivo cancelada');
    });

    // Send the request - preserve current directory path
    const uploadUrl = '/' + window.location.search;
    xhr.open('POST', uploadUrl);
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
}

// Close modal with Escape key
document.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') {
        closeCustomPathModal();
        closeSearchModal();
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

// In a real implementation, this function would make a request to the server
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

    uploadForm.addEventListener('submit', function (e) {
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
    document.getElementById('new-tab-name').focus();
}

function hideNewTabInput() {
    document.getElementById('new-tab-input-row').style.display = 'none';
}

function createClipboardTab() {
    var nameInput = document.getElementById('new-tab-name');
    var name = nameInput.value.trim();
    var protect = document.getElementById('new-tab-protect').checked;

    if (!name) { showToast('Tab name cannot be empty', 'error'); return; }
    if (!/^[a-zA-Z0-9 _-]{1,50}$/.test(name)) {
        showToast('Invalid name (only letters, numbers, spaces, - and _)', 'error');
        return;
    }

    var headers = { 'Content-Type': 'text/plain' };
    if (protect) headers['X-Tab-Token-Create'] = '1';

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
                clipboardTokenCache[name] = generatedToken;
            }
            // Clear textarea synchronously for the new (empty) tab.
            // renderClipboardTabs (called by loadClipboardTabs) already marks it active
            // via currentClipboardTab, so no extra selectClipboardTab call is needed.
            document.getElementById('shared-clipboard-textarea').value = '';
            document.getElementById('clipboard-char-count').textContent = 'chars: 0';
            setClipboardEditable(true);
            loadClipboardTabs(false);
            if (generatedToken) {
                showTokenRevealModal(generatedToken, name);
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