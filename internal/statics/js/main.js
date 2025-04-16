var input = document.getElementById('file-upload');

    
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
                input.files = ev.dataTransfer.files;
            }
        });
    } else {
        [...ev.dataTransfer.files].forEach((file, i) => {
            input.files = ev.dataTransfer.files;
        });
    }
}

function dragOverHandler(ev) {
    ev.preventDefault();
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
}

// Close modal with Escape key
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        closeCustomPathModal();
    }
});

function sortTable(n, type = 'string') {
    const table = document.querySelector(".styled-table tbody");
    const rows = Array.from(table.rows);
    const isAsc = table.dataset.sortOrder === 'asc';
    const sortOrder = isAsc ? 'desc' : 'asc';
    table.dataset.sortOrder = sortOrder;

    rows.sort((rowA, rowB) => {
        const cellA = rowA.cells[n].innerText.trim();
        const cellB = rowB.cells[n].innerText.trim();

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
                n === 3 ? 'custom-path-icon' : '';
    if (iconId) {
        const icon = document.getElementById(iconId);
        icon.className = "fa fa-sort-" + (sortOrder === 'asc' ? 'asc' : 'desc');
    }
}