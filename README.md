# Mi garaje

Libro de mantenimiento doméstico con **Go + SQLite + htmx + Pico CSS**. Sin cuentas, sin Node, sin ORM y sin proceso de compilación del frontend. HTML servido por Go, navegación y formularios mejorados con htmx; también funciona sin JavaScript.

## Uso

- Una pestaña por coche, con alta, edición y eliminación confirmada.
- Matrícula, marca, modelo, alias, km, bastidor, primera matriculación, combustible, distintivo DGT, próxima ITV, aseguradora, vencimiento y observaciones.
- Historial editable: fecha, km, precio, taller, 15 tareas habituales marcables y observaciones para otros trabajos.
- Total de gastos por coche y avisos visuales cuando ITV o seguro vencen o quedan 30 días. Fechas según Europe/Madrid. No envía notificaciones.
- Importes en céntimos enteros. Matrículas únicas sin distinguir mayúsculas ni espacios. Se admiten formatos antiguos, extranjeros y de vehículos históricos.
- Un mantenimiento con más km aumenta el kilometraje actual. Editarlo a la baja o borrarlo no reduce automáticamente el cuentakilómetros: puedes corregirlo en la ficha, siempre que no sea inferior al mayor km del historial.
- La próxima ITV se introduce manualmente según la documentación; marcar «ITV realizada» no la recalcula.

## Ejecutar en desarrollo

También se han generado `bin/mantenimiento.exe` para Windows y `bin/mantenimiento` para Ubuntu/Linux **amd64**. Para probarlo en este equipo sin instalar Go, ejecuta en PowerShell `./bin/mantenimiento.exe` y abre http://127.0.0.1:8080. Estos binarios se regeneran después de modificar el código y no se incluyen en Git.

Instala Go **1.25 o posterior** desde [go.dev](https://go.dev/dl/).

```sh
go run .
```

Abre http://127.0.0.1:8080. La base de datos se crea en `data/mantenimiento.db`. No se incluyen vehículos de ejemplo.

Configuración mediante dos variables de entorno:

| Variable | Por defecto | Uso |
| --- | --- | --- |
| `ADDR` | `127.0.0.1:8080` | Dirección y puerto HTTP |
| `DB_PATH` | `data/mantenimiento.db` | Ruta del archivo SQLite |

Para acceder desde otros equipos de casa:

```sh
ADDR=0.0.0.0:8080 go run .
```

En PowerShell: `$env:ADDR='0.0.0.0:8080'; go run .`.

## Ejecutar con Docker

Instala [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Windows o macOS) o el motor de Docker en Linux. Desde la carpeta del proyecto:

```sh
docker compose up -d --build
```

Abre http://127.0.0.1:8080. La base de datos queda en `./data` (el mismo directorio que con `go run .`). Para detener: `docker compose down`. Para ver logs: `docker compose logs -f`.

El contenedor escucha en todas las interfaces del host en el puerto 8080; mantenlo accesible solo desde tu red doméstica o VPN, ya que cualquiera que acceda puede editar o borrar los datos. No necesita un proxy inverso.

Para una copia coherente con Docker: `docker compose stop`, copia `./data` (incluido `-wal` y `-shm` si existen) y vuelve a arrancar con `docker compose start`.

## Desplegar en Ubuntu con systemd

Compila en el servidor con Go 1.25+ o lleva un binario compilado para su arquitectura. No necesita Go ni SQLite instalados para **ejecutarse**: plantillas, CSS, JavaScript y controlador SQLite están incluidos en el ejecutable.

```sh
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/mantenimiento .
```

Para compilar desde Windows hacia un Ubuntu x86-64:

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='linux'
$env:GOARCH='amd64' # usa arm64 si tu servidor es ARM
go build -trimpath -ldflags='-s -w' -o bin/mantenimiento .
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

Copia `bin/mantenimiento` y `deploy/mantenimiento.service` al servidor. Desde la carpeta que contiene ambos:

```sh
sudo useradd --system --user-group --home-dir /var/lib/mantenimiento --shell /usr/sbin/nologin mantenimiento
sudo install -d /opt/mantenimiento
sudo install -m 755 bin/mantenimiento /opt/mantenimiento/mantenimiento
sudo install -m 644 deploy/mantenimiento.service /etc/systemd/system/mantenimiento.service
sudo systemctl daemon-reload
sudo systemctl enable --now mantenimiento
sudo systemctl status mantenimiento
```

`useradd` se ejecuta solo en la primera instalación. systemd crea `/var/lib/mantenimiento` con el propietario adecuado. Visita `http://IP-DEL-SERVIDOR:8080` desde casa. El servicio escucha en todas las interfaces; mantenlo accesible solo desde tu red doméstica o VPN, ya que cualquiera que acceda puede editar o borrar los datos. Si usas UFW, permite el puerto únicamente desde tu subred doméstica. No necesita un proxy inverso.

Ver logs:

```sh
sudo journalctl -u mantenimiento -f
```

Actualizar: compila la nueva versión, detén el servicio, sustituye `/opt/mantenimiento/mantenimiento` y vuelve a iniciarlo. La base está separada del binario y se conserva. El programa aplica el esquema inicial automáticamente sin borrar datos.

## Copias de seguridad y restauración

La forma sencilla de hacer una copia coherente es detener brevemente el servicio. Con SQLite en modo WAL no copies solo el `.db` mientras está funcionando.

```sh
sudo systemctl stop mantenimiento
sudo cp -a /var/lib/mantenimiento /var/lib/mantenimiento-backup-$(date +%Y%m%d-%H%M%S)
sudo systemctl start mantenimiento
```

Guarda la copia en otro disco o equipo. Para restaurar: detén el servicio, mueve el directorio actual a una ubicación de respaldo, copia el directorio de la copia a `/var/lib/mantenimiento`, aplica `sudo chown -R mantenimiento:mantenimiento /var/lib/mantenimiento` y arranca de nuevo. Restaura el conjunto completo, incluidos los archivos `-wal` y `-shm` si los hubiera; no mezcles archivos de distintas copias.

## Estructura y comprobaciones

```text
main.go                 Servidor, rutas, plantillas y recursos embebidos
db.go                   Esquema SQLite y consultas
handlers.go             CRUD y validación
templates/layout.html   Pantallas y formularios
static/                 Pico CSS, htmx, estilos y JS mínimo
deploy/                 Servicio systemd
Dockerfile              Imagen multi-etapa
docker-compose.yml      Puerto 8080 y volumen ./data
```

```sh
go test ./...
go vet ./...
```

Se comprueban operaciones reales contra SQLite temporal: CRUD, validación, céntimos, aislamiento entre vehículos, borrado en cascada, persistencia, HTML escapado y rechazo de escrituras desde otros orígenes.

Pico CSS 2.1.1 y htmx 2.0.8 están incluidos con sus licencias. Solo la primera compilación necesita Internet para descargar dependencias de Go. Referencias: [htmx](https://htmx.org/docs/), [Pico CSS](https://picocss.com/docs), [modernc SQLite](https://pkg.go.dev/modernc.org/sqlite).
