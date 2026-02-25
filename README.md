# sofascore-scrapper

Scraper en Go que obtiene los eventos deportivos de [Sofascore](https://www.sofascore.com/es/) y los almacena en una base de datos MariaDB usando GORM.

## Descripción

El scraper:

1. Abre `https://www.sofascore.com/es/` con un navegador sin cabeza (Chromium vía `chromedp`) para ejecutar el JavaScript de la página.
2. Espera a que el contenido se renderice y obtiene el HTML resultante.
3. Parsea los elementos con clase `debpTI` que son hijos de elementos con clase `mdDown:pt_sm` usando `goquery`.
4. Guarda la información de cada evento deportivo en la tabla `sport_events` de MariaDB mediante GORM.

## Estructura del proyecto

```
.
├── main.go              # Punto de entrada
├── models/
│   └── event.go         # Modelo GORM SportEvent
├── database/
│   └── db.go            # Conexión a MariaDB con GORM
├── scraper/
│   └── scraper.go       # Lógica de scraping y parsing HTML
├── Dockerfile
├── docker-compose.yml
└── go.mod / go.sum
```

## Variables de entorno

| Variable               | Valor por defecto | Descripción                                          |
|------------------------|-------------------|------------------------------------------------------|
| `DB_HOST`              | `localhost`       | Host de la base de datos                             |
| `DB_PORT`              | `3306`            | Puerto de la base de datos                           |
| `DB_USER`              | `root`            | Usuario de la base de datos                          |
| `DB_PASSWORD`          | *(vacío)*         | Contraseña de la base de datos                       |
| `DB_NAME`              | `sofascore`       | Nombre de la base de datos                           |
| `CHROMIUM_NO_SANDBOX`  | *(no definido)*   | Poner `true` para habilitar `--no-sandbox` en Docker |

## Ejecución con Docker Compose

```bash
docker-compose up --build
```

Esto levanta un contenedor de MariaDB y ejecuta el scraper.

## Ejecución local

Requisitos: Go 1.24+, Chromium instalado, MariaDB disponible.

```bash
export DB_HOST=localhost
export DB_PASSWORD=tu_contraseña
go run .
```

## Modelo de datos

La tabla `sport_events` almacena:

| Campo        | Tipo         | Descripción                              |
|--------------|--------------|------------------------------------------|
| `id`         | uint         | ID autoincremental (GORM)                |
| `data_id`    | varchar(100) | Valor del atributo `data-id` del enlace  |
| `sport`      | varchar(100) | Deporte                                  |
| `tournament` | varchar(255) | Torneo o liga                            |
| `home_team`  | varchar(255) | Equipo local                             |
| `away_team`  | varchar(255) | Equipo visitante                         |
| `home_score` | varchar(20)  | Marcador del equipo local                |
| `away_score` | varchar(20)  | Marcador del equipo visitante            |
| `status`     | varchar(100) | Estado del partido (e.g. "En juego")     |
| `start_time` | varchar(50)  | Hora de inicio                           |
| `raw_text`   | text         | Texto completo del elemento scrapeado    |
| `scraped_at` | datetime     | Marca de tiempo del scraping             |