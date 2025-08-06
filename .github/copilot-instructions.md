# Instrucciones para GitHub Copilot en Upgopher

## Visión General del Proyecto

Upgopher es un servidor web simple escrito en Go que proporciona funcionalidades para compartir y gestionar archivos. Diseñado como una alternativa a los servidores de archivos basados en Python, Upgopher ofrece una solución portable y compilable para múltiples plataformas.

## Arquitectura y Componentes Principales

### Estructura del Proyecto

- `upgopher.go`: Archivo principal que contiene toda la lógica de la aplicación
- `internal/statics/`: Paquete que gestiona los recursos estáticos embebidos
  - `statics.go`: Maneja la carga de templates, CSS y JavaScript
  - `templates/index.html`: Interfaz de usuario principal
  - `css/styles.css`: Estilos de la aplicación
  - `js/main.js`: Funcionalidad JavaScript del cliente

### Componentes Principales

1. **Sistema de Manejo de Archivos**:
   - Carga/descarga de archivos con manejo de rutas seguras (`isSafePath`)
   - Navegación por directorios con codificación base64 para rutas
   - Generación de archivos ZIP para descarga de directorios completos

2. **Interfaz Web**:
   - UI basada en HTML/CSS/JS con recursos embebidos
   - Funcionalidad de arrastrar y soltar para carga de archivos
   - Visualización y ordenación de listas de archivos

3. **Seguridad**:
   - Autenticación básica opcional (usuario/contraseña)
   - Soporte para HTTPS con certificados personalizados o autogenerados
   - Validación de rutas para prevenir path traversal

4. **Funcionalidades Adicionales**:
   - Portapapeles compartido para intercambiar texto
   - Búsqueda de texto en archivos
   - Rutas personalizadas/acortadas para archivos
   - Opción para mostrar/ocultar archivos ocultos

## Flujos de Trabajo de Desarrollo

### Compilación

El proyecto utiliza GoReleaser para la gestión de compilaciones multiplataforma:

```bash
# Compilación local simple
go build

# Compilación con GoReleaser (vista previa)
goreleaser release --snapshot --clean
```

### Arquitectura de Handlers HTTP

Los handlers HTTP siguen un patrón común:
```go
func customHandler(dir string) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    // Verificación de ruta segura
    // Lógica del handler
    // Respuesta HTTP
  }
}
```

### Convenciones Importantes

1. **Codificación de Rutas**: Las rutas de archivo se codifican en base64 para su transferencia segura en URLs:
   ```go
   encodedPath := base64.StdEncoding.EncodeToString([]byte(filePath))
   decodedPath, _ := base64.StdEncoding.DecodeString(encodedPath)
   ```

2. **Validación de Seguridad**: Todas las rutas de usuario deben validarse con `isSafePath`:
   ```go
   isSafe, err := isSafePath(dir, fullPath)
   if err != nil || !isSafe {
     http.Error(w, "Bad path", http.StatusForbidden)
     return
   }
   ```

3. **Manejo de Recursos Estáticos**: Los recursos estáticos se incrustan mediante la directiva `//go:embed`:
   ```go
   //go:embed static/favicon.ico
   var favicon embed.FS
   ```

## Puntos de Integración y Extensión

1. **Añadir Nuevas Funcionalidades HTTP**:
   - Crear un nuevo handler en `upgopher.go`
   - Registrarlo en la función `main()`
   - Considerar la autenticación si está habilitada

2. **Modificar la Interfaz de Usuario**:
   - Editar `internal/statics/templates/index.html` para cambios en HTML
   - Editar `internal/statics/css/styles.css` para cambios en estilo
   - Editar `internal/statics/js/main.js` para cambios en comportamiento

3. **Ampliar Capacidades de Compilación**:
   - Modificar `.goreleaser.yml` para configurar opciones de compilación o packaging

## Configuración y Opciones

El servidor admite múltiples flags de línea de comandos para su configuración:
```
-port int        Puerto del servidor (default 9090)
-dir string      Directorio para almacenar archivos (default "./uploads")
-user string     Usuario para autenticación básica
-pass string     Contraseña para autenticación básica
-ssl             Habilitar HTTPS
-cert string     Ruta al certificado SSL
-key string      Ruta a la clave privada SSL
-q               Modo silencioso (sin logs)
-disable-hidden-files  Deshabilitar mostrar archivos ocultos
```
