# CHANGELOG

## 2026-10-27 — tracer.20: `segundo_factor` (TOTP) en el login del `bff` (E8-T5)

**Resumen:** sube el pin de `pf-pkg-contracts` a `v0.1.0-tracer.20` y propaga `segundo_factor` en `iniciarSesion`. tracer.20 agrega `CredencialesEntrada.segundo_factor` (TOTP, `nullable`, opcional V1) al contrato del `bff`, espejo del campo que ya existe en `identidad`. El `bff` lo transporta; el IdP (Keycloak) lo valida (DEC-031/DEC-071, ADR 13).

**Para quién:** para `pf-web-frontend-internal`, que necesita enviar el TOTP en el login (E8-T5). Sin la línea de traducción el campo se descartaría en silencio en el `bff`.

**Cambios:**
- `go.mod`/`go.sum`: `api-client-go` de `v0.1.0-tracer.17` a `v0.1.0-tracer.20`.
- `internal/agregacion/handlers.go`: `IniciarSesion` copia `cuerpo.SegundoFactor` a `idclient.CredencialesEntrada` (la traducción es campo a campo entre paquetes generados distintos, por eso hay que añadir la línea explícita).

**Nota:** el bump trae también al contrato del `bff` la ruta `listarClientes` y `Asignacion.estado`; ninguno se cablea aquí. `Asignacion.estado` es inerte (el `bff` no proyecta asignación) y `listarClientes` va en su propio carril (depende de que `catalogos` lo implemente).

## 2026:10:23 — Passthrough del token hacia operacion e identidad (arreglo del actor de descarga)

**Resumen:** El `bff` ahora propaga el Bearer del usuario aguas abajo en TODAS sus aristas de usuario, no solo en la de `mensajeria`. Antes, los adaptadores HTTP de `operacion` e `identidad` se construían sin propagar el token: toda llamada salía SIN `Authorization`. Esto arregla un bug real en la descarga del expediente (E6-T3, ya en `-d`): `registrarDescargaViaje` deja el actor del rastro de auditoría a partir de `claims.Subject` en `operacion`; sin el token, el actor quedaba vacío o `operacion` respondía 401. El doc-comment del adaptador ya prometía esta propagación; ahora el código la cumple.

**Para quién:** para cualquier usuario cuyo request pase por el `bff` hacia `operacion` (crear/listar/seguir viaje, bitácora, auditoría, registrar descarga) o haga logout en `identidad`. `operacion` deriva rol/alcance/actor del token (DEC-053, E1-T7); sin el Bearer no puede.

**Mecánica — por qué el contexto y no un parámetro del puerto:** la arista `bff→operacion` (e `identidad`) tiene DOS adaptadores del mismo puerto. El HTTP (standalone, I5) cruza un proceso y DEBE reenviar el token como header. El EN PROCESO de `core` NO cruza proceso: los claims ya viajan en el `ctx` de la petición y `operacion` los lee con `auth.ClaimsFromContext`. Meter `bearer string` en la firma del puerto obligaría al adaptador en proceso a aceptar un dato que no usa y tocaría `pf-svc-core` (I4). En su lugar, el handler guarda el Bearer en el `ctx` (`internal/tokenctx`) y el adaptador HTTP lo lee e inyecta con un `RequestEditorFn`; el adaptador en proceso lo ignora. La firma del puerto no cambia y `core` no se toca.

**Qué se propaga y qué no** (verificado contra el servidor, ADR 26 / DEC-053/054):
- `operacion`: las seis (crearViaje, listarViajes, obtenerViaje, listarBitacora, consultarAuditoria, registrarDescargaViaje) → **propagan el Bearer**.
- `identidad`: `revocarSesion` (logout) → **propaga el Bearer**. `emitirSesion`/`refrescarSesion` son públicas. `obtenerSesion`/`listarSesionesRevocadas` son consultas de servicio en background (proyección de revocación, ADR 26 §5): NO hay usuario en ese flujo, así que NO se propaga un token de usuario.

**Cambios:**
- `internal/tokenctx/tokenctx.go` (nuevo): paquete hoja con `Con(ctx, bearer)` y `De(ctx)` para llevar el Bearer en el contexto. Documenta por qué es contexto y no parámetro.
- `internal/agregacion/handlers.go`: `CrearViaje`, `ListarViajes`, `SeguirViaje`, `DescargarViaje` (y sus llamadas en cadena) y `CerrarSesion` envuelven el `ctx` con `tokenctx.Con(r.Context(), r.Header.Get("Authorization"))`.
- `adapters/http/operacion.go`: `conBearerOperacion(ctx)` devuelve un `RequestEditorFn` que reenvía el Bearer del ctx; se pasa en las seis llamadas.
- `adapters/http/identidad.go`: `conBearerIdentidad(ctx)` análogo; se pasa solo en `revocarSesion`.

**Pruebas:**
- `adapters/http/passthrough_test.go` (nuevo): con un `HttpRequestDoer` fake que captura el header, se verifica que `operacion.ObtenerViaje` reenvía el `Authorization` del ctx, que sin Bearer no inventa cabecera, y que `identidad.RevocarSesion` propaga el Bearer.
- `internal/agregacion/handlers_test.go`: `TestSeguirViajePropagaBearerAOperacion` cierra el extremo handler (el `Authorization` del request llega al ctx que recibe el puerto); `fakeOperacion` captura el bearer del ctx.

**Verificado:** `gofmt -l .` limpio, `go build ./...`, `go vet ./...`, `go test ./...` en verde y `arch-test: OK`.

**Decisiones / notas:** (1) La arista a `mensajeria` conserva su `bearer string` explícito: es una arista sin gemelo en proceso, el patrón no aplica ahí. (2) Brecha de contrato reportada (no corregida aquí): `refrescarSesion` es pública en runtime pero `identidad.yaml` no le declara `security: []` (solo `emitirSesion`). Es trabajo de contrato.

## 2026:10:23 — Carril interno de emergencia: panel de atención (E7/tracer.17)

**Resumen:** Se agrega el lado interno de emergencia al `bff`, bajo el prefijo nuevo `/api/v1/panel`. Reacción (y supervisor/coordinador con alcance) ahora puede listar los casos de un viaje con la vista interna completa y ATENDERLOS: reclamar, liberar el reclamo, transferir, resolver y cerrar. Completa el punta-a-punta de E7: el carril cliente (activar/nota/listar) ya existía; este es el carril del operador que atiende.

**Para quién:** para el operador interno que opera la emergencia desde el panel. `bff` propaga el Bearer del usuario y reenvía a `mensajeria`, que autoriza por los claims (`PuedeAtenderEmergencia`, ADR 21 §6) y proyecta la vista interna completa (origen, actor real, reclamo). El cliente no alcanza estas rutas: su token es de audiencia cliente y `mensajeria` responde según audiencia.

**Cambios:**
- `internal/agregacion/ports.go`: `MensajeriaPort` se amplía con `ReclamarEmergencia`, `LiberarReclamoEmergencia`, `TransferirEmergencia`, `ResolverEmergencia` y `CerrarEmergencia`. Cada una recibe el `bearer` como las tres existentes.
- `adapters/http/mensajeria.go`: implementación de los cinco métodos sobre el cliente generado de `mensajeria` (tracer.17), reusando `conBearer`, el presupuesto y la traducción de errores. `LiberarReclamoEmergencia` valida éxito por status 2xx (la respuesta no lleva cuerpo).
- `internal/agregacion/handlers.go`: handlers del panel (`ListarEmergenciasPanel` + las cinco acciones). A diferencia del portal cliente, NO traducen la respuesta: la vista interna es la del contrato de `mensajeria` sin recortar (el contrato de `bff` solo define la vista cliente). `versionEsperadaDe` lee el query `version_esperada` para liberar; `viajeYEmergenciaDe` parsea los path values comunes.
- `bff.go`: `Routes()` monta las seis rutas del panel bajo `/api/v1/panel/viajes/{id}/emergencias...`. El prefijo nuevo existe porque la ruta `.../emergencias` de `/api/v1/viajes` ya la ocupa el portal cliente y un `ServeMux` no admite dos handlers para el mismo patrón.

**Pruebas:**
- `bff_test.go`: `TestRutaNoDeContratoNoMontada` se ajusta: `/api/v1/panel` a secas sigue sin resolver patrón (el panel solo monta subrutas).
- `internal/agregacion/emergencia_test.go`: `fakeMensajeria` extendido con los cinco métodos; `peticionPanel` (claims de audiencia interna, path value `rid` y query). Casos: listar reenvía la vista completa con origen; reclamar propaga `version_esperada` y devuelve el reclamo; liberar lee `version_esperada` del query y responde 204; `version_esperada` no numérica es 400 y no llega al puerto; resolver propaga la nota; cerrar devuelve el caso cerrado.

**Verificado:** `gofmt -l .` limpio, `go build ./...`, `go vet ./...`, `go test ./...` en verde y `arch-test: OK`.

**Decisiones:** (1) El panel propaga el token y deja autorizar a `mensajeria`; `bff` no valida rol, igual que el resto de sus reenvíos. (2) La vista interna reenvía tipos de `mensajeria` sin traductor, porque la vista interna es la completa y no hay recorte que hacer (los traductores cliente existen solo para quitar campos).

**Pendiente (no bloqueante):** unificar el passthrough del token en TODAS las aristas del `bff` (hoy solo `mensajeria` lo propaga; `operacion`/`identidad` siguen con doer nil).

## 2026:10:22 — Descarga del expediente de un viaje (AI27-80 / E6-T3)

**Resumen:** Se implementa `GET /api/v1/viajes/{id}/descarga` (`descargarViaje`), la descarga del expediente de UN viaje para usuarios internos (DEC-022 #7). `bff` compone el expediente —ficha técnica + histórico de bitácora + auditoría relevante— leyendo SIEMPRE de `operacion` (la fuente exacta y actual), NUNCA de `reportes` (analítico y eventualmente consistente). Tras componer con éxito, le pide a `operacion` que DEJE el rastro de auditoría de la descarga vía `registrarDescargaViaje`: el rastro lo escribe `operacion` en su transacción, no `bff` (que no tiene base de datos).

**Para quién:** para el usuario interno (operativo, supervisor, administrador…) que exporta el expediente de un viaje desde el panel. El cliente no puede: recibe `403` antes de que se componga dato alguno. El alcance fino (qué viajes ve cada operativo) lo resuelve `operacion`; `bff` propaga su `403`/`404` tal cual.

**Cambios:**
- `go.mod`/`go.sum`: pin de `api-client-go` en `v0.1.0-tracer.17` (reutilizado de AI27-59). Trae `bff.DescargaViaje`/`FichaTecnicaDescarga`/`EntradaBitacoraDescarga`/`RegistroAuditoriaDescarga` y, en `operacion`, `listarBitacora`, `consultarAuditoria` y `registrarDescargaViaje`.
- `internal/agregacion/ports.go`: `OperacionPort` se amplía con `ListarBitacora`, `ConsultarAuditoria` (ambas por página, con cursor opaco) y `RegistrarDescargaViaje` (sin cuerpo: el actor sale de los claims aguas abajo). `ObtenerViaje` ya existía para la ficha.
- `adapters/http/operacion.go`: adaptador HTTP de los tres métodos nuevos sobre el cliente generado, con el presupuesto de `operacion` y la traducción de errores a la taxonomía de la plataforma.
- `internal/descarga/descarga.go` (nuevo): `Componer` es una función PURA que arma `bff.DescargaViaje` a partir del detalle del viaje, la bitácora y la auditoría ya leídas. Solo serializa campos contratados; las listas nunca son `nil`.
- `internal/agregacion/handlers.go`: handler `DescargarViaje`. Verifica audiencia INTERNA con `ClaimsDe(ctx).Audiencia` ANTES de componer nada (cliente → `403`); agota la paginación de bitácora y auditoría; compone; y llama a `RegistrarDescargaViaje` EXACTAMENTE una vez por descarga exitosa. Si `operacion` falla al dejar el rastro, la descarga no se entrega. Introduce el PRIMER check de audiencia en un handler de `bff`.
- `internal/errores/errores.go`: nuevo error `Prohibido` (403) de `bff` con `NewProhibido`/`EsProhibido`/`CodigoProhibido`. La taxonomía de la plataforma colapsa 403 en 401 (`CategoryNoAutorizado`), pero el contrato de `bff` separa `Prohibido` (403) de `NoAutorizado` (401); `DeStatus(403)` ahora devuelve `Prohibido` y `escribirError` lo mapea a 403.
- `bff.go`: `Routes()` monta `GET /api/v1/viajes/{id}/descarga` envuelta en `protegido`. NO se monta ninguna ruta de lote: la exportación de listados es V2 (DEC-023).
- Exclusiones duras (DEC-009/DEC-039): el expediente NO contiene cuenta espejo ni `clave_amago`. No existen en el modelo V1 de `operacion` ni en el contrato de la descarga, así que no hay nada que filtrar; una prueba a nivel HTTP afirma que el cuerpo nunca los contiene.

**Pruebas:**
- `bff_test.go`: `descargarViaje` añadida a la lista de rutas contratadas (`operacionesFaseA`); se verifica además que no existe ruta de lote (`POST /api/v1/descargas`, `POST /api/v1/viajes/descargas`).
- `internal/agregacion/descarga_test.go`: rechazo por audiencia (cliente → `403`, sin tocar `operacion` ni dejar rastro); `401` sin claims; `403` de alcance y `404` de viaje inexistente propagados desde `operacion`; descarga exitosa que compone el expediente y deja el rastro EXACTAMENTE una vez (verificado en el fake); agotamiento de la paginación de bitácora y auditoría; fallo al registrar → no se entrega el expediente; y la descarga no contiene datos excluidos.
- `internal/descarga/descarga_test.go`: `Componer` puro mapea ficha/bitácora/auditoría, mantiene las listas no-`nil` y nunca serializa datos excluidos.

**Verificado:** `go build ./...`, `go vet ./...` y `go test ./...` en verde. `bff` no tiene base de datos propia: el rastro de auditoría de la descarga lo escribe `operacion`.

**Nota / brecha reportada:** el contrato de `operacion` (`operacion.yaml`) declara `consultarAuditoria` (`GET /api/v1/viajes/{id}/auditoria`) y el cliente generado lo trae, pero el servidor Go de `pf-svc-operacion` todavía NO monta esa ruta (su módulo `auditoria` es solo de escritura por interceptor). `bff` compone contra el contrato (correcto); hasta que `operacion` monte la lectura de auditoría, esa sección del expediente llegará vacía o `operacion` responderá `404`, que `bff` propaga. Es trabajo de `operacion`, no de `bff`.

**Archivos modificados:**
- `go.mod`, `go.sum`
- `internal/agregacion/ports.go`, `internal/agregacion/handlers.go`, `internal/agregacion/handlers_test.go`, `internal/agregacion/descarga_test.go` (nuevo)
- `internal/descarga/descarga.go` (nuevo), `internal/descarga/descarga_test.go` (nuevo)
- `internal/errores/errores.go`
- `adapters/http/operacion.go`
- `bff.go`, `bff_test.go`

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
