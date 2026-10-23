# CHANGELOG

## 2026:10:22 18:20

**Resumen:** Se implementa la vista curada de incidencias para el cliente (AI27-59 / E2-T3) en `GET /api/v1/viajes/{id}` (`seguirViaje`). `bff` traduce el tipo interno de cada incidencia activa a una de las cinco categorías del cliente, sin filtrar nunca el término interno (DEC-006). Se cierra así la brecha de contrato que el scaffold dejaba documentada: `operacion` ya expone `incidencias_activas` en `ViajeDetalle` (tracer.16), y `bff` deja de devolver la lista de incidencias vacía a propósito.

**Para quién:** para el `cliente` que sigue su viaje desde el portal, que ahora ve la categoría curada + estado (`en_atencion`/`resuelta`) + marca de tiempo de cada incidencia abierta, sin exponer jamás el nombre técnico del catálogo operativo. Y para quien mantenga `bff`: el mapeo vive en un módulo puro y probado por enumeración del catálogo completo.

**Cambios:**
- `go.mod`/`go.sum`: pin de `api-client-go` de `v0.1.0-tracer.10` a `v0.1.0-tracer.16`. El salto es aditivo para lo que `bff` consume; trae `operacion.ViajeDetalle.incidencias_activas` (`[]IncidenciaActiva` con `tipo_interno`, `estado`, `abierta_en`) y confirma `bff.IncidenciaCurada` + `ViajeSeguimiento.incidencias`.
- `internal/tracking/incidencias.go` (nuevo): módulo de mapeo PURO. Las cinco categorías EXACTAS como constantes (`Incidencia de seguridad`, `Accidente/percance`, `Falla mecánica/de equipo`, `Retraso/desvío en ruta`, `Pérdida de contacto`) y el mapa cerrado `tipo_interno → categoría` según `docs/propuestas/propuesta-clasificacion-estatus-v1.md` §3 (DEC-006). `Categoria` devuelve `(categoría, ok)` y nunca el término interno; `incidenciasCuradas` compone la lista del cliente.
- `internal/tracking/tracking.go`: `Componer` ahora rellena `incidencias` a partir de `ViajeDetalle.incidencias_activas` (categoría mapeada + estado + marca de tiempo). Se eliminó el marcador `incidenciasPendientes()` del scaffold; su brecha quedó cerrada. `impacto_eta` queda `nil`: `operacion` no lo expone por incidencia en tracer.16 (no se inventa).
- La incidencia aparece en cuanto el operador la marca, sin paso extra de "publicar al cliente" (DEC-021): es la misma lectura de `condicion_viaje` vía `operacion`. Al resolverse en `operacion`, sale de `incidencias_activas` y deja de verse en la siguiente lectura, sin acción adicional.
- `internal/tracking/tracking_test.go`: pruebas del módulo puro. Enumera los 18 tipos internos y verifica que cada uno mapea a su categoría y que ninguno se filtra; caso de doble dimensión (`Sabotaje GPS` → una sola `Incidencia de seguridad`, nunca dos entradas ni el nombre técnico); traducción de estado `resuelta`; y la desaparición de la incidencia tras resolverse. Reemplaza al antiguo `TestComponerIncidenciasVacias` (brecha) por `TestComponerSinIncidenciasActivas`.
- `internal/agregacion/handlers_test.go`: pruebas de comportamiento a nivel HTTP vía `httptest`. Para cada tipo interno, confirma que no aparece en el cuerpo JSON crudo del cliente; `Sabotaje GPS` da una sola entrada curada; y tras resolver, la siguiente lectura ya no trae la incidencia.

**Verificado:** `go build ./...`, `go vet ./...` y `go test -count=1 ./...` en verde; `gofmt` limpio en los archivos tocados. Verificación de mutación: cambiar la categoría de `Sabotaje GPS` puso `TestSabotajeGpsEsUnaSolaIncidenciaDeSeguridad` en rojo; mutación revertida. Las pruebas afirman comportamiento, no implementación.

**Archivos modificados:**
- `go.mod`, `go.sum`
- `internal/tracking/incidencias.go` (nuevo)
- `internal/tracking/tracking.go`, `internal/tracking/tracking_test.go`
- `internal/agregacion/handlers_test.go`

## 2026:10:19

**Resumen:** Se sube el pin de `pf-pkg-contracts` de `v0.1.0-tracer.4` a `v0.1.0-tracer.10`. El salto es 100% aditivo en los clientes generados que bff consume (sin líneas eliminadas ni cambios de firma entre las dos olas), así que no toca ningún handler ni tipo existente. Desbloquea de hecho E6-T3 (AI27-80, Gonzalo): la operación `descargarViaje` y sus schemas (`DescargaViaje`, `FichaTecnicaDescarga`, …) entraron al contrato en tracer.11 y recién ahora están en el cliente Go que bff resuelve; sin este bump no se podían consumir. No monta la ruta de descarga (es trabajo de E6-T3, de otro dueño): solo pone el contrato al día.

**Para quién:** para quien tome E6-T3 sobre bff y para cualquier consumo futuro de la superficie de fase (b): el cliente generado deja de estar siete olas atrás del contrato vivo.

**Cambios:**
- `go.mod`/`go.sum`: `api-client-go` a `v0.1.0-tracer.10`.

**Verificado:** `go build`/`vet` en verde, `gofmt` limpio, `go test ./...` en verde, `arch-test@v0.1.1 OK`.

## 2026:10:07 15:37

**Resumen:** Pruebas de comportamiento round-trip para los handlers de agregacion de bff. Cierra el unico hueco de cobertura del scaffold: la capa de handlers tenia solo la prueba de montaje de rutas, que pasaria aunque cada handler devolviera la forma equivocada.

**Para quien:** para quien mantenga bff. Ahora un cambio que rompa el status, la forma de respuesta, el reenvio aguas abajo o la traduccion de error de cualquier operacion de fase (a) sale en rojo en vez de pasar silencioso.

**Cambios:**
- `internal/agregacion/handlers_test.go`: 11 pruebas que llaman a cada handler por una peticion HTTP (como su usuario real) y afirman status y cuerpo JSON decodificado contra valores literales, mas la entrada que recibio el puerto falso.
- Cubre: `iniciarSesion`/`refrescarSesion` (201/200, mapeo de sesion, reenvio de credenciales/token), `cerrarSesion` (204, revoca el sesion_id del token no de un parametro; 401 sin claims), `crearViaje` (201, traduccion bff->operacion sin fijar asignacion), `listarViajes` (200, reenvio de cursor), `seguirViaje` (200, hito compuesto que nunca filtra el token tecnico; 400 id invalido; 404 ausencia genuina).
- Prueba de traduccion de error: aguas-abajo degradado -> 503, nunca 404 (bff.md (d)).
- Puertos falsos `fakeIdentidad` y `fakeOperacion` que registran su entrada.

**Verificado:**
- `gofmt -l .` limpio; `go vet ./...`, `go build ./...`, `go test ./...` pasan (`internal/agregacion` paso de 0 a 11 pruebas).
- `GOFLAGS=-mod=readonly go build ./...`, `go build ./cmd/api` y `arch-test@v0.1.1 .` en verde.
- Verificacion de mutacion: romper `CrearViaje` (201->200) y la etiqueta de hito de `seguirViaje` puso cada prueba en rojo; ambas mutaciones revertidas. Las pruebas afirman comportamiento, no implementacion.

**Archivos modificados:**
- `internal/agregacion/handlers_test.go`

## 2026:10:07 14:54

**Resumen:** Scaffolding inicial del microsistema `bff` (fase a de `bff.yaml`), contrato primero contra `pf-pkg-contracts@v0.1.0-tracer.3` y `pf-pkg-platform-go@v0.1.0`. Compila, prueba y pasa arch-test. Las brechas de contrato se reportan, no se inventan.

**Cambios:**
- `go.mod` (`module github.com/AI27-Tech/pf-svc-bff`, `go 1.27`) con versiones exactas de ambos paquetes; sin `replace` ni `go.work` versionado.
- Superficie del módulo (`bff.go`): `New`/`Routes`/`Workers`. Sin `Consultas()` (bff no tiene aristas entrantes). `Pools` es un struct vacío (bff no es dueño de datos). `Workers()` vacío hasta que la raíz inyecte el bus.
- Puertos de salida de fase (a) en `internal/agregacion/ports.go` (`IdentidadPort`, `OperacionPort`), re-exportados como alias en `ports.go`. Tipados solo con tipos generados del contrato.
- Handlers de las seis operaciones (`iniciarSesion`, `cerrarSesion`, `refrescarSesion`, `crearViaje`, `listarViajes`, `seguirViaje`) en `internal/agregacion`.
- Composición pura de hitos V1 (`internal/tracking`, E2-T2/E2-T3, ADR 27): traducción estado técnico → hito, colapso multi-parada, con pruebas por estado.
- Autorización y revocación (`internal/autorizacion`, ADR 26): envoltura de `auth.Middleware`, proyección de tombstones idempotente que falla cerrado, con pruebas de rechazo de sesión revocada.
- Adaptación de errores aguas abajo a la taxonomía de la plataforma (`internal/errores`): degradado/roto, nunca no encontrado.
- Adaptadores HTTP (`adapters/http/identidad.go`, `operacion.go`) sobre los clientes generados, con presupuesto por dependencia.
- `cmd/api/main.go`: arranque standalone (I5) con adaptadores HTTP, verificador local de JWT, sin BD ni migraciones.
- CI (`.github/workflows/ci.yml`): build/vet/test, build de `cmd/api`, arch-test fijado a `@v0.1.1` (honra `shared_packages` y `roles`).
- `README.md` (español) y `.gitignore`.

**Verificado:**
- `gofmt -l .` limpio; `go vet ./...`, `go build ./...`, `go test ./...` pasan.
- `GOFLAGS=-mod=readonly go build ./...` pasa (go.sum completo).
- `go build ./cmd/api` compila el arranque standalone.
- `arch-test@v0.1.1 .` → OK (aristas a `operacion` e `identidad` permitidas).

**Brechas de contrato reportadas (no resueltas):** `stale` (contradicción E2-T2/ADR 20 vs `operacion.yaml`), incidencia curada (E2-T3, sin superficie en `ViajeDetalle`), `catalogo.actualizado` (consumidor contradictorio), `GET /api/v1/panel` (no existe en el contrato), forma de sesión de los frontends (E8-T3). Ver `README.md`.

**Archivos modificados:**
- `go.mod`, `go.sum`, `bff.go`, `ports.go`, `bff_test.go`, `.gitignore`
- `cmd/api/main.go`
- `adapters/http/identidad.go`, `adapters/http/operacion.go`
- `internal/agregacion/ports.go`, `internal/agregacion/handlers.go`
- `internal/tracking/tracking.go`, `internal/tracking/tracking_test.go`
- `internal/autorizacion/autorizacion.go`, `internal/autorizacion/autorizacion_test.go`
- `internal/errores/errores.go`
- `.github/workflows/ci.yml`, `README.md`

## 2026:10:06 12:55

**Resumen:** Se agregó la plantilla de Pull Request del backend, adaptada al stack Go de AI27.

**Cambios:**
- Se agregó `.github/pull_request_template.md` con secciones de invariantes de arquitectura (I1-I5, contrato primero), base de datos, eventos/bus, verificación (go build/vet/test, arch-test, gofmt), CHANGELOG y checklist de convenciones.

**Archivos modificados:**
- `.github/pull_request_template.md`

## 2026:10:06 12:48

**Resumen:** Se agregó la regla de steering de CHANGELOG y este archivo, antes de que el repo tenga código, para que la convención esté vigente desde el inicio.

**Cambios:**
- Se agregó `.kiro/steering/changelog.md` (`inclusion: always`): toda modificación del repo debe registrarse aquí, en español, con marca de tiempo, resumen, cambios y archivos modificados.
- Se creó `CHANGELOG.md`.

**Archivos modificados:**
- `.kiro/steering/changelog.md`
- `CHANGELOG.md`
