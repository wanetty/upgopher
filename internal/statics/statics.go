package statics

import (
	"fmt"
)

func GetTemplates(table string, backButton string, downloadButton string, disableHiddenFiles bool) string {
	html := getHTML()
	css := getCSS()
	javascript := getJS()
	disabled := "display: flex;"
	if disableHiddenFiles {
		disabled = "display: none;"
	}

	result := fmt.Sprintf(html, css, table, disabled, backButton, downloadButton, javascript)
	return result
}

// CSS Code
func getCSS() string {
	cssCode := `
    <style>
        body {
            background-color: #f5f5f5;
            font-family: sans-serif;
        }
        h1, h2 {
            color: #2d2d2d;
            text-align: center;
        }

        .styled-table {
            margin-left: auto;
            margin-right: auto;
            border-collapse: collapse;
            font-size: 0.9em;
            font-family: sans-serif;
            min-width: 100px;
            box-shadow: 0 0 20px rgba(0, 0, 0, 0.15);
        }
        .styled-table thead tr {
            background-color: #009879;
            color: #ffffff;
            text-align: center;
        }
        .styled-table th,
        .styled-table td {
            padding: 12px 70px;
        }
        .styled-table tbody tr {
            border-bottom: 1px solid #dddddd;
        }

        .styled-table tbody tr:nth-of-type(even) {
            background-color: #f3f3f3;
        }

        .styled-table tbody tr:last-of-type {
            border-bottom: 2px solid #009879;
        }
        .styled-table tfoot tr {
            background-color: #f3f3f3;
            border-top: 2px solid #009879;
        }
        .styled-table tbody tr.active-row {
            font-weight: bold;
            color: #009879;
        }
        td a {
            color: #3A8C5B;
            font-weight: bold;
        }
        .tdspe {
            width: 20px
        }
        td a:hover {
            color: #45BC75;
        }
        .btn {
            display: inline-block;
            background-color: #009879;
            border: none;
            color: #fff;
            padding: 10px;
            text-align: center;
            text-decoration: none;
            font-size: 14px;
            margin: -5px 4px;
            border-radius: 5px;
            cursor: pointer;
        }
        .btn:hover {
            background-color: #2C4534;
        }
        form {
            margin: 20px auto;
            text-align: center;
        }
        input[type=file] {
            width: 350px;
            max-width: 100%;
            color: #444;
            padding: 5px;
            background: #fff;
            border-radius: 10px;
            display: none
          }
        input[type=file]::file-selector-button {
            margin-right: 20px;
            border: none;
            background: #009879;
            padding: 10px 20px;
            border-radius: 10px;
            color: #fff;
            cursor: pointer;
            transition: background .2s ease-in-out;
        }

        input[type=file]::file-selector-button:hover {
        background: #0d45a5;
        }
        input[type=submit]:hover {
            background-color: #2C4534;
        }
        .center {
            display: block;
            margin-left: auto;
            margin-right: auto;
            width: 175px;
        }
        .form-group label {
            display: inline-block;
            margin-right: 10px;
        }
        .form-group input[type="file"] {
            display: inline-block;
            width: auto;
            margin-right: 10px;
        }
        .form-group button {
            display: inline-block;
            vertical-align: middle;
        }
        #drop_zone {
            width: 745px;
            display: block;
            margin-left: auto;
            margin-right: auto;
            border: 1px dashed black;
            vertical-align: middle;
        }
        .code-box {
            margin: 5px auto; 
            background-color: #ffffff;
            color: #009879;
            border-radius: 5px;
            font-family: 'Courier New', monospace;
            width: 700px;
            overflow-x: auto;
            white-space: pre;
            box-shadow: 0 0 20px rgba(0, 0, 0, 0.15);
            width: 80%; /* Ajusta el ancho según tus necesidades */
        }
        
        .line-number {
            color: #5c6370;
            display: inline-block;
            width: 30px;
            user-select: none;
        }
        
        .highlight {
            color: #e06c75;
        }
        .checkbox-container {
            display: block;
            position: relative;
            padding-left: 24px;
            margin-bottom: 12px;
            cursor: pointer;
            font-size: 0.95em;
            user-select: none;
            margin: 0 auto;
            color: #2d2d2d; /* Color de texto consistente */
        }

        .checkbox-container.center {
            display: flex;
            justify-content: center;
            align-items: center;
        }

        /* Hide the browser's default checkbox */
        .checkbox-container input {
            position: absolute;
            opacity: 0;
            cursor: pointer;
            height: 0;
            width: 0;
        }

        .icon-checkbox {
            position: absolute;
            top: 0;
            left: 0;
            height: 20px;
            width: 20px;
            display: flex;
            justify-content: center;
            align-items: center;
        }

        /* Create a custom checkbox */
            .checkmark {
            position: absolute;
            top: 0;
            left: 0;
            height: 16px;
            width: 16px;
            background-color: #ccc;
        }

        /* On mouse-over, add a grey background color */
            .checkbox-container:hover input ~ .checkmark {
            background-color: #ccc;
        }

        /* When the checkbox is checked, add a blue background */
        .checkbox-container input:checked ~ .checkmark {
            background-color: #009879;
        }

        /* Create the checkmark/indicator (hidden when not checked) */
        .checkmark:after {
            content: "";
            position: absolute;
            display: none;
        }

        /* Show the checkmark when checked */
        .checkbox-container input:checked ~ .checkmark:after {
            display: block;
        }

        /* Style the checkmark/indicator */
        .checkbox-container .checkmark:after {
            left: 5px;
            top: 2px;
            width: 3px;
            height: 8px;
            border: solid white;
            border-width: 0 3px 3px 0;
            -webkit-transform: rotate(45deg);
            -ms-transform: rotate(45deg);
            transform: rotate(45deg);
        }
       /* Estilos actualizados para el modal */
        .modal-overlay {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            width: 100vw;
            height: 100vh;
            background-color: rgba(0, 0, 0, 0.5);
            z-index: 1000;
            justify-content: center;
            align-items: center;
        }

        .modal {
            background: #ffffff;
            border-radius: 8px;
            padding: 24px;
            width: 90%;
            max-width: 500px;
            position: relative;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            margin: 20px;
        }

        .modal-header {
            margin-bottom: 24px;
        }

        .modal-title {
            color: #333333;
            margin: 0;
            font-size: 1.5rem;
            font-weight: 600;
        }

        .form-group {
            margin-bottom: 20px;
        }

        .form-group label {
            display: block;
            margin-bottom: 8px;
            color: #333333;
            font-weight: 500;
        }

        .form-group input {
            width: 100%;
            padding: 10px;
            border: 1px solid #e1e1e1;
            border-radius: 4px;
            font-size: 14px;
            transition: border-color 0.3s ease;
            box-sizing: border-box;
        }

        .form-group input:focus {
            outline: none;
            border-color: #009879;
        }

        .modal-footer {
            display: flex;
            justify-content: flex-end;
            gap: 12px;
            margin-top: 24px;
        }

        .btn-modal {
            padding: 8px 16px;
            border-radius: 4px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            border: none;
            transition: background-color 0.2s ease;
        }

        .btn-cancel {
            background-color: #f5f5f5;
            color: #333333;
        }

        .btn-cancel:hover {
            background-color: #e5e5e5;
        }

        .btn-create {
            background-color: #009879;
            color: white;
        }

        .btn-create:hover {
            background-color: #008568;
        }

        /* Animation */
        @keyframes modalSlideIn {
            from {
                transform: translateY(-20px);
                opacity: 0;
            }
            to {
                transform: translateY(0);
                opacity: 1;
            }
        }

        .modal {
            animation: modalSlideIn 0.3s ease-out;
        }
    </style>
    `
	return cssCode
}

// HTML Code
func getHTML() string {
	html := `
    <!DOCTYPE html>
    <html>
        <head>
            <meta charset="UTF-8">
            <title>Uploaded Files</title>
            <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.min.css">
            <style>
            %s
            </style>
        </head>
        <body>
            <div>
                <img class="center"  src="/static/logopher.webp">
            </div>
            <div>
            <table style="margin-top: 20px;" class="styled-table">
                <thead>
                    <tr>
                        <th onclick="sortTable(0)">Name <i id="name-icon" class="fa"></i></th>
                        <th>
                            Permissions
                        </th>
                        <th onclick="sortTable(2, 'number')">Size <i id="size-icon" class="fa"></i></th>
                        <th onclick="sortTable(3)">Custom Path <i id="custom-path-icon" class="fa"></i></th>
                        <th>
                            Actions
                        </th>
                    </tr>
                </thead>
                <tbody id="fileTableBody">
                %s
                </tbody>
                <tfoot>
                    <tr>
                        <td colspan="5">
                            <div style="%s  justify-content: space-between; align-items: center;">
                                <label class="checkbox-container" for="showAlertCheckbox">Show Hidden Files
                                    <input type="checkbox" id="showAlertCheckbox">
                                    <span class="checkmark"></span>
                                </label>
                                <div id="additional-footer-options">
                                
                                </div>
                            </div>
                        </td>
                    </tr>
                </tfoot>
            </table>
            </div>
            <br>
            <div class="code-box">
                <div><span class="line-number">1</span>curl -X POST -F "file=@[/path/to/file]" http://[SERVER]:[PORT]/</div>
            </div>
            <br>
            <div id="drop_zone" ondrop="dropHandler(event);" ondragover="dragOverHandler(event);">
                <h1>Upload a File</h1>
            </div>
            <form method="POST" class="form-group" enctype="multipart/form-data">
                <input type="file" name="file" id="file-upload"><input type="submit" class="btn" value="Upload">
            </form>

            <div style="display: flex; flex-direction: row; justify-content: center; align-items: center; height: 100px;">
                <div style="display: flex; justify-content: center; align-items: center; height: 100px;">
                    %s
                </div>
                <div style="display: flex; justify-content: center; align-items: center; height: 100px;">
                    %s
                </div>
            </div>
            <!-- Modal para la creación de rutas personalizadas -->
            <div id="customPathModal" class="modal-overlay">
                <div class="modal">
                    <div class="modal-header">
                        <h2 class="modal-title">Create Custom Path</h2>
                    </div>
                    <form id="pathForm">
                        <div class="form-group">
                            <label for="originalPath">Original Path:</label>
                            <input type="text" id="originalPath" readonly>
                        </div>
                        <div class="form-group">
                            <label for="customPath">Custom Path:</label>
                            <input type="text" id="customPath" placeholder="Enter custom path" required>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn-modal btn-cancel" onclick="closeCustomPathModal()">Cancel</button>
                            <button type="button" class="btn-modal btn-create" onclick="createCustomPath()">Create</button>
                        </div>
                    </form>
                </div>
            </div>
            <script>
                %s
            </script>
     </body>
    </html>
`
	return html
}

// Javascript Code
func getJS() string {
	javascript := `
    var input = document.getElementById('file-upload');

    document.addEventListener('DOMContentLoaded', function() {
        const checkbox = document.getElementById('showAlertCheckbox');

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
    });

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
        const urlWithParam = baseUrl + "/raw/" + decodedPath + "/"+ fileName;
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

    // Cerrar modal al hacer clic fuera de él
    window.onclick = function(event) {
        if (event.target == document.getElementById('customPathModal')) {
            closeCustomPathModal();
        }
    }

    // Cerrar modal con la tecla Escape
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
    `
	return javascript
}
