# Roadmap — Auditoría de integración `flutter-apptv` ↔ `sofascore-scrapper`

> Documento vivo. Cada entrada referencia un issue concreto en GitHub.
> Última auditoría: 2026-08-25.

## Contexto

Se auditó la integración entre el cliente Flutter (`E:\Projects\IPTV\flutter-apptv`) y el
backend Go que sirve el panel de eventos deportivos (`E:\Projects\IPTV\sofascore-scrapper`).
El único canal compartido es HTTP + protobuf binario sobre `/api/app/v1/*`, autenticado
por header `APP-XIPTV`.

## Hallazgos (resumen)

| # | Severidad | Repo afectado | Resumen | Issue |
|---|-----------|---------------|---------|-------|
| H1 | Alta | flutter-apptv | Logos de equipos externos no cargan (concatenación rota de URLs) | [jeriveromartinez/flutter-apptv#4](https://github.com/jeriveromartinez/flutter-apptv/issues/4) |
| H2 | Alta | flutter-apptv | `DashboardNotifier` construye `EventsApi` que nunca usa — feature "muerta" | [jeriveromartinez/flutter-apptv#5](https://github.com/jeriveromartinez/flutter-apptv/issues/5) |
| H3 | Media | ambos | `SofaScoreEvent` proto sin `status_type` — backend lo envía, cliente lo ignora | [jeriveromartinez/flutter-apptv#6](https://github.com/jeriveromartinez/flutter-apptv/issues/6) |
| H4 | Media | flutter-apptv | `DeviceApi.register` decodifica la respuesta `Device` como `DeviceRegisterRequest` | [jeriveromartinez/flutter-apptv#7](https://github.com/jeriveromartinez/flutter-apptv/issues/7) |
| H5 | Media | ambos | Drift masivo entre `flutter-apptv/proto/api.proto` y `sofascore-scrapper/proto/api.proto` | [jeriveromartinez/sofascore-scrapper#28](https://github.com/jeriveromartinez/sofascore-scrapper/issues/28) |
| H6 | Baja | flutter-apptv | `EventsApi` nunca se libera en Riverpod (leak de `http.Client`) | [jeriveromartinez/flutter-apptv#8](https://github.com/jeriveromartinez/flutter-apptv/issues/8) |
| H7 | Baja | sofascore-scrapper | `GET /current-events` ignora silenciosamente `limit` fuera de rango | [jeriveromartinez/sofascore-scrapper#29](https://github.com/jeriveromartinez/sofascore-scrapper/issues/29) |
| H8 | Baja | sofascore-scrapper | `logoURLForAPI` tiene caso especial frágil (`/api/app/v1` literal) | [jeriveromartinez/sofascore-scrapper#30](https://github.com/jeriveromartinez/sofascore-scrapper/issues/30) |
| H9 | Baja | flutter-apptv | Slider accede a `sliderItems[sliderIndex]` sin protección de bounds | [jeriveromartinez/flutter-apptv#9](https://github.com/jeriveromartinez/flutter-apptv/issues/9) |
| H10 | Baja | sofascore-scrapper | Filtro `date` en admin trata entrada como UTC sin avisar | [jeriveromartinez/sofascore-scrapper#31](https://github.com/jeriveromartinez/sofascore-scrapper/issues/31) |
| H11 | Baja | flutter-apptv | `debugPrint` con copy-paste de otro archivo en `ProtoApi._checkResponse` | [jeriveromartinez/flutter-apptv#10](https://github.com/jeriveromartinez/flutter-apptv/issues/10) |

## Principios de corrección

1. **Propagar, no podar.** Si una pieza de código existe y consume una feature, mantenerla
   y conectarla bien — no borrarla. Ejemplos:
   - `EventsApi` en `DashboardNotifier` debe **mostrar eventos** en una sección del
     dashboard, no eliminarse.
   - `status_type` debe **propagarse** al UI como badge "EN VIVO" / "FINALIZADO".
   - El campo `Device` de respuesta debe **usarse** (p. ej. para guardar el `deviceId`
     asignado por el backend).

2. **Una fuente de verdad para el contrato.** El `.proto` vive en `sofascore-scrapper/`
   y se sincroniza a `flutter-apptv/` mediante `make proto` o equivalente. Ningún
   consumidor debería editar el `.proto` de su lado.

3. **Test de wire-format por cada mensaje compartido.** Ya existen en el backend
   (`internal/gen/api/contract_test.go`); agregar el equivalente en Flutter o un test
   de contrato cross-repo en CI.

## Fases

### Fase 0 — Hotfix visible (1 día)
- [**flutter-apptv#4**](https://github.com/jeriveromartinez/flutter-apptv/issues/4) Logos externos rotos. Fix en una función pura (`EventsApi.listNext`) +
  test unitario. Sin tocar backend.

### Fase 1 — Propagar features existentes (2-3 días)
- [**flutter-apptv#5**](https://github.com/jeriveromartinez/flutter-apptv/issues/5) Conectar `EventsApi` al `DashboardNotifier` para mostrar una fila
  "En vivo y próximos" debajo del slider. Decidir destino de la actual fila de
  favoritos si queda redundante (preferentemente mantener ambas).
- [**flutter-apptv#6**](https://github.com/jeriveromartinez/flutter-apptv/issues/6) + parte de **#28** Una vez sincronizado el proto, propagar `status_type` al
  `DashboardSliderItem` (campo nuevo) y al widget del slider como badge/color.
- [**flutter-apptv#7**](https://github.com/jeriveromartinez/flutter-apptv/issues/7) Cambiar `post<DeviceRegisterRequest, ...>` por `post<Device, ...>` y
  propagar el `device.id` resultante a un provider si hace falta.

### Fase 2 — Higiene y contrato (1-2 días)
- [**flutter-apptv#8**](https://github.com/jeriveromartinez/flutter-apptv/issues/8) Dispose de `EventsApi` en ambos providers (`SliderNotifier`,
  `DashboardNotifier`). Reusar el método ya existente en `ProtoApi`.
- [**sofascore-scrapper#29**](https://github.com/jeriveromartinez/sofascore-scrapper/issues/29) Validar `limit` en el handler: devolver 400 si `limit > 6` o
  `limit < 1`, en lugar de ignorar.
- [**sofascore-scrapper#30**](https://github.com/jeriveromartinez/sofascore-scrapper/issues/30) Refactor `logoURLForAPI`: normalizar primero, luego prefijar.
  Cubrir con casos de test (`""`, `"/"`, `"//cdn/x"`, etc.).

### Fase 3 — Sincronización de proto (2-3 días)
- [**sofascore-scrapper#28**](https://github.com/jeriveromartinez/sofascore-scrapper/issues/28) Sincronizar `flutter-apptv/proto/api.proto` con
  `sofascore-scrapper/proto/api.proto`. Regenerar `api.pb.dart`,
  `api.pbenum.dart`, `api.pbjson.dart`. Eliminar el `go_package` del
  `.proto` de Flutter (sólo importa al backend). Agregar script `make proto`.

### Fase 4 — Pulido (1 día)
- [**flutter-apptv#9**](https://github.com/jeriveromartinez/flutter-apptv/issues/9) Clamp `sliderIndex` antes de leer `sliderItems`.
- [**sofascore-scrapper#31**](https://github.com/jeriveromartinez/sofascore-scrapper/issues/31) Documentar el comportamiento UTC del filtro `date` o aceptar TZ.
- [**flutter-apptv#10**](https://github.com/jeriveromartinez/flutter-apptv/issues/10) Corregir el `debugPrint` errante.

## Criterio de "done" por fase

- **Fase 0**: build verde, slider muestra escudos externos.
- **Fase 1**: dashboard muestra fila de eventos; status_type visible en el slider;
  `device.id` se propaga al menos a un consumer.
- **Fase 2**: sin leaks observables en tests de Riverpod; handler retorna 400 en
  uso indebido; tests de `logoURLForAPI` verdes.
- **Fase 3**: hashes SHA256 de ambos `api.proto` coinciden; suite de Flutter
  compila; tests de contrato Go verdes.
- **Fase 4**: lint limpio; mensajes de log correctos.

## Riesgo y mitigación

| Riesgo | Mitigación |
|--------|-----------|
| Regenerar proto rompe código Flutter que asume IDs viejos | Mantener compatibilidad por número de campo (nunca reordenar ni reusar) |
| Cambiar respuesta de `Device` impacta `DeviceApi.register` | Hacer la corrección en commit atómico con su test |
| `limit` validation cambia contrato público | Cobertura de test e2e del flujo app |

## Recomendación de arranque

Empezar por **Fase 0 / H1** porque:

- Es el bug más visible (logos rotos en el slider del dashboard).
- Está contenido a un único archivo (`events.dart`) + un test unitario.
- No requiere tocar el backend ni regenerar el proto.
- Desbloquea la confianza del equipo antes de entrar a Fase 1.

Una vez verde, saltar a **Fase 1 / H2** (propagar `EventsApi` al
`DashboardNotifier`), siguiendo el principio de no podar features.
