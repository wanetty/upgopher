.PHONY: help build test test-race test-coverage test-short lint clean run

# Variables
BINARY_NAME=upgopher
COVERAGE_FILE=coverage.out

help: ## Muestra esta ayuda
	@echo "Comandos disponibles:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Compila el proyecto
	@echo "Compilando..."
	go build -o $(BINARY_NAME) -ldflags="-s -w"
	@echo "✅ Compilación exitosa: ./$(BINARY_NAME)"

test: ## Ejecuta todos los tests
	@echo "Ejecutando tests..."
	go test -v ./...

test-race: ## Ejecuta tests con detección de race conditions
	@echo "Ejecutando tests con race detector..."
	go test -race -v ./...

test-coverage: ## Ejecuta tests con cobertura
	@echo "Ejecutando tests con cobertura..."
	go test -cover -coverprofile=$(COVERAGE_FILE) ./...
	@echo "\nCobertura detallada:"
	go tool cover -func=$(COVERAGE_FILE)
	@echo "\nPara ver cobertura en HTML: go tool cover -html=$(COVERAGE_FILE)"

test-short: ## Ejecuta tests rápidos (sin tests largos)
	@echo "Ejecutando tests rápidos..."
	go test -v -short ./...

lint: ## Ejecuta análisis estático (go vet)
	@echo "Ejecutando linters..."
	go vet ./...
	@echo "✅ Análisis completado"

clean: ## Elimina binarios y archivos temporales
	@echo "Limpiando..."
	rm -f $(BINARY_NAME)
	rm -f $(COVERAGE_FILE)
	@echo "✅ Limpieza completada"

run: build ## Compila y ejecuta el servidor (puerto 9090)
	@echo "Iniciando servidor..."
	./$(BINARY_NAME)

run-auth: build ## Ejecuta el servidor con autenticación (user: admin, pass: admin)
	@echo "Iniciando servidor con autenticación..."
	./$(BINARY_NAME) -user admin -pass admin

run-ssl: build ## Ejecuta el servidor con HTTPS (certificado auto-firmado)
	@echo "Iniciando servidor con HTTPS..."
	./$(BINARY_NAME) -ssl

# Targets de desarrollo
dev: test-race lint ## Ejecuta tests con race detector y linters
	@echo "✅ Checks de desarrollo completados"

ci: test-race lint build ## Pipeline completo de CI
	@echo "✅ Pipeline CI completado exitosamente"
