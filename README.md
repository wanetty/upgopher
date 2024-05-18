# Upgopher

<p align="center"><img width=250 alt="Logo" src="https://github.com/wanetty/upgopher/blob/main/static/logopher.png"></p>

[![Go](https://github.com/wanetty/upgopher/actions/workflows/go.yml/badge.svg)](https://github.com/wanetty/upgopher/actions/workflows/go.yml)

This is a simple Go web server that allows users to upload files and view a list of the uploaded files. The server can be run locally or deployed to a remote server.

This project tries to replace all file servers that use python, since there are always problems with libraries. Sometimes we want it to be on a remote pc and go gives you the possibility of cross-platform compilation and to work anywhere...

![Exmaple Photo](./static/example.png)
## Features
* Users can upload files by selecting a file and clicking the "Upload" button
* Uploaded files are stored in the "uploads" directory by default, but the directory can be changed using the -dir flag
* Users can view a list of the uploaded files by visiting the root URL
* Basic authentication is available to restrict access to the server. To use it, set the -user and -pass flags with the desired username and password.
* Traffic via HTTPS.
* Possibility to browse through folders and upload files...


## Installation


### Automatically

Just run this command in your terminal with go installed.
```bash
go install github.com/wanetty/upgopher@latest
```

### Releases

Go to the [releases](https://github.com/wanetty/upgopher/releases) section and get the one you need.

### Manual

Just build it yourself

```bash
git clone https://github.com/wanetty/upgopher.git
cd upgopher
go build 
```
### Docker

```bash
podman build . -t upgopher
podman run --name upgopher -p 9090:9090  upgopher
```

## Usage

### Help Output:

```bash
./upgopher -h
Usage of ./upgopher:
  -cert string
        certificado para HTTPS
  -dir string
        directory path (default "./uploads")
  -key string
        clave privada para HTTPS
  -pass string
        password for authentication
  -port int
        port number (default 9090)
  -tls
        utilizar HTTPS
  -user string
        username for authentication
```

## License
This project is licensed under the MIT License. See the LICENSE file for details.

## Info
For more information, you can find me on Twitter as [@gm_eduard](https://twitter.com/gm_eduard/).
