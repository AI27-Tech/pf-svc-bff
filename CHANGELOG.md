# CHANGELOG

## 2026-10-30 — Cursor de dominio en el bootstrap de la conversación del cliente (E9-T8, ADR 28 §2)

**Para quién:** el portal cliente, que al abrir una conversación recibe ahora `ultima_secuencia` y con ella pide el catch-up (`listarMensajes?despues_de=`), sin derivar el cursor del último mensaje listado.

**Resumen:** sube el pin de contratos a `tracer.29` y mapea `c.UltimaSecuencia` de `mensajeria` al campo `ultima_secuencia` de `ConversacionCliente`, que la vista de cliente recortaba. Es el cursor de dominio que ADR 28 §2 exige en la respuesta de bootstrap. `mensajeria` siempre lo calcula (0 si la conversación no tiene mensajes).

**Contexto:** cierra el gap de §2 de la **opción A de DT-01** (`docs/DEUDA-TECNICA.md` del repo umbral). El push vivo por SSE (opción C) sigue aplazado.

**Cambios:**
- `internal/agregacion/handlers.go`: `aConversacionCliente` mapea `UltimaSecuencia: int64(c.UltimaSecuencia)`; comentario actualizado (ya no "descarta secuencias").
- `internal/agregacion/conversacion_test.go`: `TestObtenerConversacionRecortaYPropagaToken` afirma `ultima_secuencia == 7`.
- `go.mod`/`go.sum`: pin de contratos tracer.28→.29.

**Verificación:** `gofmt`, `go vet ./...`, `go build ./...`, `go test ./...`, `arch-test` en verde. Bump tracer.28→.29 sin renombres de enum (forma aditiva).

## 2026-10-30 — Ruta interna de transición de viaje (E8-T14, cierre del servicio)

**Para quién:** el frontend interno, que ya cierra el viaje contra el bff real (`POST /api/v1/viajes/{id}/transiciones`) en vez de un mock, y completa la matriz end-to-end de la tarea 12 (400 / 401 / 403 / 409 con código / 409 genérico / carrera). Para quien mantiene el bff: una ruta interna espejo de la de cliente, con audiencia invertida y respuesta de detalle interno.

**Resumen:** monta la superficie interna `transicionarViaje` de `bff.yaml` (hasta ahora fuera por scaffolding de fase (a)). Exige audiencia Interna (un actor Cliente recibe 403; los clientes transicionan por `/transiciones-cliente`). `operacion` sigue siendo la única autoridad: valida la arista, la facultad de cierre, el `motivo_cierre` (DEC-113) y los conflictos (409 con código); el bff no replica el grafo. A diferencia de la vista de cliente, la respuesta es el detalle interno (`ViajeDetalle`: estado canónico + condiciones), no la vista de seguimiento que oculta ambos (DEC-006).

**Fix de causa raíz incluido:** `aTransicionOperacion` descartaba `motivo_cierre`. El bug estaba latente porque el cliente nunca cierra; la ruta interna lo destapa. Sin el fix, toda transición a `CERRADO` que pasa por el bff llegaría a `operacion` sin motivo y sería un 400. El adapter es compartido por ambas rutas, así que el fix cubre cualquier cierre futuro.

**Cambios:**
- `internal/agregacion/handlers.go`: handler `TransicionarViaje` (check audiencia Interna, reenvío, detalle interno recompuesto); mapper nuevo `aViajeDetalle` (`opclient.ViajeDetalle` → `bffclient.ViajeDetalle`); `aTransicionOperacion` ahora propaga `MotivoCierre`.
- `bff.go`: ruta montada; comentario de `Deps.Operacion` corregido (ya no dice "sin transiciones internas").
- `internal/agregacion/ports.go`: doc de `OperacionPort` corregido.
- `bff_test.go`: `transicionarViaje` en la prueba de contrato.
- `internal/agregacion/handlers_test.go`: audiencia cliente → 403, sin claims → 401, 200 con detalle interno (estado y condiciones visibles), reenvío de `motivo_cierre`, 409 propagado.

**Alcance:** solo `/transiciones`. `/detalle`, `/eta`, `/asignacion` quedan para sus propias historias.

**Verificación:** `gofmt`, `go vet ./...`, `go build ./... && go build ./cmd/api`, `go test ./...`, `arch-test` en verde. Sin bump de pin de contratos (los tipos ya estaban en `tracer.28`).

## 2026-10-30 — Tipos de servicio para el wizard de solicitud del cliente (E9-T4)

**Para quién:** el portal cliente, que ya puede poblar el selector de tipo de servicio del wizard de solicitud (E1-HU-001) contra el bff real, sin llamar a `catalogos` directo.

**Resumen:** sube el pin de contratos a `tracer.28` y monta `GET /api/v1/catalogos/tipos-servicio` (`listarTiposServicio`). El handler reenvía a `catalogos.listarValoresCatalogo` con el tipo fijo `tipos_servicio` (el cliente no elige familia), filtra a los valores `activo` y recorta a la vista mínima `TipoServicioCliente` (`codigo`, `etiqueta`). Solo audiencia Cliente: un actor interno recibe 403 antes de reenviar.

**Por qué el tipo es fijo:** de las cinco familias de `TipoCatalogo`, solo `tipos_servicio` es de cara al cliente; las otras (`zonas_riesgo`, etc.) son operativas internas. El bff no expone el `{tipo}` genérico al cliente. Por DEC-017, rutas/marcas/kit son texto libre, no catálogo.

**Cambios:**
- `internal/agregacion/ports.go`: `CatalogosPort` gana `ListarTiposServicio`.
- `adapters/http/catalogos.go`: adaptador que fija `catclient.TiposServicio` y reenvía el cursor.
- `internal/agregacion/handlers.go`: handler `ListarTiposServicio` (check audiencia Cliente) y traductor `aTiposServicioCliente` (filtra activos, recorta).
- `bff.go`: ruta montada.
- `bff_test.go`: `listarTiposServicio` en la prueba de contrato.
- `internal/agregacion/catalogos_test.go`: nuevo. Afirma el filtro de inactivos, el recorte y el rechazo de audiencia interna.

**Verificación:** `gofmt`, `go vet ./...`, `go build ./... && go build ./cmd/api`, `go test ./...`, `arch-test` en verde. Bump tracer.27→.28 sin renombres de enum (forma aditiva).

## 2026-10-30 — Ola C: 403 y código de máquina de conflictos propagados al cliente

**Para quién:** el portal cliente y el panel interno, que ahora reciben el código específico del conflicto (no `conflicto` genérico) y un 403 real cuando les falta la facultad. Para quien mantiene el bff: desaparece el tipo `Prohibido` local; el 403 vive en la plataforma.

**Resumen:** sube el pin de `pf-pkg-platform-go` a `v0.1.8` y el de `pf-pkg-contracts` a `v0.1.0-tracer.27`, y adopta la taxonomía de la Ola C. El `Prohibido` local del bff converge a `CategoryProhibido` de la plataforma; el 409 aguas abajo con código de máquina (p. ej. `cierre_anticipado_requiere_incidencia` de operacion, `conversacion_solo_lectura` de mensajeria) se propaga con ese código en vez de aplastarse a `conflicto`.

**Cambios:**
- `internal/errores`: se elimina el tipo `Prohibido`, `NewProhibido`, `EsProhibido` y `CodigoProhibido` (convergen a `platformerrors`). `DeStatus` pasa a delegar en `DeRespuesta(op, status, codigo)`; el 403 usa `platformerrors.NewProhibido` y el 409 con código usa `NewConflictoCodigo`.
- `adapters/http/operacion.go`: `TransicionarViaje` lee `resp.JSON409.Codigo` y lo propaga.
- `adapters/http/mensajeria.go`: `EnviarMensaje` propaga el código del 409 (`conversacion_solo_lectura`).
- `internal/agregacion/handlers.go`: los tres `NewProhibido` locales pasan a `platformerrors.NewProhibido`; `escribirError` se simplifica a `AsCategory` + `CodigoDe` (un solo camino, sin caso especial de 403).
- `internal/errores/errores_test.go`: nuevo. Afirma que el 409 con código lo conserva, el 409 sin código cae en `conflicto`, y el 403 da categoría Prohibido.
- Pin de plataforma a `v0.1.8` y de contratos a `tracer.27` (tracer.26 quedó a medias; .27 es su republicación, forma idéntica).

**Verificación:** `gofmt`, `go vet ./...`, `go build ./... && go build ./cmd/api`, `go test ./...` y `arch-test` en verde. Sin renombres de enum (§8): el bump de contratos .26→.27 no cambió ninguna forma.

## 2026-10-30 — E9-T8 (B.2): conversación del viaje para el portal cliente

**Resumen:** sube el pin de `pf-pkg-contracts` a `v0.1.0-tracer.26` y monta las tres rutas de la conversación del viaje para audiencia Cliente: `GET /api/v1/viajes/{id}/conversacion` (`obtenerConversacionViaje`), `GET` y `POST /api/v1/viajes/{id}/conversacion/mensajes` (`listarMensajesViaje`, `enviarMensajeViaje`). Segunda familia de la arista `bff→mensajeria` tras emergencias. La conversación se identifica por el viaje (DEC-029): el `{id}` de ruta es el viaje, que viaja como id a `mensajeria`.

**Forma de la vista de cliente:** los handlers exigen audiencia Cliente (un actor interno recibe 403 antes de reenviar), propagan el Bearer y recortan la respuesta de `mensajeria` a la vista de cliente:
- `ConversacionCliente`: `viaje_id`, `estado`, `atendida` (derivada de `atendida_en != null`, DEC-014). Descarta `cliente_id`, bandeja, reclamo, pool y secuencias.
- `MensajeCliente`: `id`, `secuencia`, `cuerpo`, `enviado_en` (← `creado_en`), `de_cliente` (derivado de `audiencia_autor == Cliente`). NUNCA `actor_real` ni rol interno; AI27 se presenta como "AI27 Operations" (DEC-021).
- Envío idempotente por `client_message_id`; el bff no manda `version_reclamo_esperada` ni `motivo_override` (superficie interna).

**Paginación por secuencia:** `listarMensajesViaje` reenvía `antes_de`/`despues_de`/`limite` tal cual a `mensajeria.listarMensajes` y conserva `hay_mas` en la respuesta (paginación por secuencia de dominio, no cursor opaco: habilita el catch-up del chat tras reconexión, Addendum §9). Esto es lo que corrigió `tracer.26`.

**Cambios:** `MensajeriaPort` gana `ObtenerConversacion`/`ListarMensajes`/`EnviarMensaje`; adaptador `MensajeriaHTTP` los implementa (mismo patrón de presupuesto/Bearer/traducción de error que emergencias); handlers `ObtenerConversacionViaje`/`ListarMensajesViaje`/`EnviarMensajeViaje` con helper `soloCliente` y mapeos `aConversacionCliente`/`aMensajeCliente`/`aPaginaMensajesCliente`; rutas en `Routes()`; tres entradas en la prueba de contrato `operacionesFaseA`; tests de handler nuevos (`conversacion_test.go`) que cubren el recorte, la propagación de secuencia y el rechazo de audiencia interna.

**Shortcut:** `enviarMensajeViaje` responde 201 también en el reintento idempotente (`mensajeria` distingue 200); el cuerpo es el mismo mensaje. Actualizar si el frontend llega a ramificar sobre 200 vs 201.

**Bump de pin (git-workflow §8):** subir el pin a `tracer.26` revirtió el nombre de las constantes del enum `IncidenciaCurada.estado`: `EnAtencion`/`Resuelta` → `IncidenciaCuradaEstadoEnAtencion`/`...Resuelta` (la ola añadió otro enum que deshizo la colisión de nombre corto). Valor de wire sin cambio. Se actualizaron los dos usos en `internal/tracking/tracking_test.go`. Sin cambio de comportamiento.

**Verificación:** `gofmt`, `go vet ./...`, `go build ./... && go build ./cmd/api`, `go test ./...` y `arch-test` en verde.

## 2026-10-29 — E8-T14: pin de contrato a tracer.24 (motivo_cierre tipado)

**Resumen:** sube el pin de `pf-pkg-contracts` de `tracer.23` a `tracer.24`. La ola trae `motivo_cierre` (enum) en `TransicionEntrada` y los códigos de 409 del cierre. El `bff` no requiere cambios de código: `transicionarViaje` ya reenvía el cuerpo `TransicionEntrada` completo a `operacion`, que valida el motivo (A.1/A.2 de E8-T14); el `bff` solo transporta el campo nuevo.

**Nota:** los códigos de 409 del cierre que el contrato ahora declara (`cierre_anticipado_solo_por_incidencia`, etc.) todavía NO llegan distinguibles al cliente: `internal/errores.DeStatus` aplana el 409 a `conflicto`. Propagarlos es la tarea de plataforma diferida (A3/B1, DEC-111), no esta entrada.

**Verificación:** `gofmt`, `go vet ./...`, `go build ./...`, `go test ./...` en verde tras el bump. Sin renombres de enum (git-workflow §8).

## 2026-10-29 — E9-T7: transición del viaje para audiencia Cliente (cancelar / rechazar)

**Resumen:** sube el pin de `pf-pkg-contracts` de `v0.1.0-tracer.20` a `v0.1.0-tracer.23` y monta la ruta nueva `POST /api/v1/viajes/{id}/transiciones-cliente` (`transicionarViajeCliente`). El handler exige audiencia Cliente (un actor interno recibe 403 antes de reenviar) y delega en `operacion.transicionarViaje`, que es la única autoridad de transición: valida la arista, aplica el aislamiento por tenant (el cliente solo su viaje, DEC-112; viaje ajeno → 404) y la facultad (solo `CANCELADO`/`RECHAZADO_POR_CLIENTE`, DEC-111) más el motivo obligatorio. `operacion.transicionarViaje` devuelve el `Viaje` plano, así que el handler recompone la vista de seguimiento del cliente (`ViajeSeguimiento`) con `ObtenerViaje`, igual que `seguirViaje`; nunca expone el estado canónico ni las condiciones (DEC-006).

**Para quién:** desbloquea E9-T7 en el portal cliente (`pf-web-frontend-client`), que ahora puede construir los botones de cancelar y rechazar sobre la ruta nueva. Cierra la secuencia de tres pasos de DEC-111: guarda en `operacion` (PR #60) → contrato `tracer.23` (PR #82) → esta superficie en el `bff`.

**Cambios:** `OperacionPort` gana `TransicionarViaje`; adaptador `OperacionHTTP` lo implementa (POST con cuerpo, patrón de `CrearViaje`); handler `TransicionarViajeCliente` con check de audiencia y recomposición de la vista; mapeo `aTransicionOperacion` (traduce `a_estado` string del bff a `EstadoViaje` de operacion); ruta en `Routes()` y entrada en la prueba de contrato `operacionesFaseA`.

**Bump de pin (git-workflow §8):** subir el pin renombró las constantes del enum `IncidenciaCurada.estado` en el cliente generado: `IncidenciaCuradaEstadoEnAtencion`/`...Resuelta` → `EnAtencion`/`Resuelta` (el generador dejó el nombre corto al añadirse otros enums con el valor `en_atencion` en olas intermedias). El valor de wire (`"en_atencion"`/`"resuelta"`) no cambió. Se actualizó el único uso afectado, en `internal/tracking/tracking_test.go`. Sin cambio de comportamiento.

**Nota 401 vs 403:** el contrato declara `403` para la arista no permitida, pero `operacion` responde `401` en ese caso (la plataforma aún no tiene un `403` propio; pendiente ratificado, DEC-111). El check de audiencia del propio `bff` sí devuelve 403 (vía `errores.NewProhibido`).

**Verificación:** `gofmt`, `go vet ./...`, `go build ./...` y `go test ./...` en verde. Tests nuevos del handler: audiencia interna → 403, sin claims → 401, cliente cancela → 200 con reenvío traducido y vista de seguimiento, error de operacion (404) propagado.

## 2026-10-27 — tracer.20: `segundo_factor` (TOTP) en el login del `bff` (E8-T5)

**Resumen:** sube el pin de `pf-pkg-contracts` a `v0.1.0-tracer.20` y propaga `segundo_factor` en `iniciarSesion`. tracer.20 agrega `CredencialesEntrada.segundo_factor` (TOTP, `nullable`, opcional V1) al contrato del `bff`, espejo del campo que ya existe en `identidad`. El `bff` lo transporta; el IdP (Keycloak) lo valida (DEC-031/DEC-071, ADR 13).

**Para quién:** para `pf-web-frontend-internal`, que necesita enviar el TOTP en el login (E8-T5). Sin la línea de traducción el campo se descartaría en silencio en el `bff`.

**Cambios:**
- `go.mod`/`go.sum`: `api-client-go` de `v0.1.0-tracer.17` a `v0.1.0-tracer.20`.
- `internal/agregacion/handlers.go`: `IniciarSesion` copia `cuerpo.SegundoFactor` a `idclient.CredencialesEntrada` (la traducción es campo a campo entre paquetes generados distintos, por eso hay que añadir la línea explícita).

**Nota:** el bump trae también al contrato del `bff` la ruta `listarClientes` y `Asignacion.estado`; ninguno se cablea aquí. `Asignacion.estado` es inerte (el `bff` no proyecta asignación) y `listarClientes` va en su propio carril (depende de que `catalogos` lo implemente).

## 2026-10-27 — tracer.20: selector de clientes `GET /api/v1/clientes` (E8-T11)

**Resumen:** el `bff` proyecta `catalogos.listarClientes` como `GET /api/v1/clientes` para poblar el selector de cliente al crear un viaje por Operación. Reenvía el cursor y propaga el Bearer; `catalogos` devuelve SOLO clientes activos (DEC-101 #1). Mismo patrón de proyección que `listarViajes`→`operacion`.

**Para quién:** para `pf-web-frontend-internal`, que necesita el selector al crear un viaje desde el panel (E8-T11).

**Alcance (DEC-026 #3 / DEC-101 #2):** el filtro de alcance del `jefe_operativo` NO lo aplica `catalogos` (es hoja, sin mecanismo de alcance); DEC-101 #2 lo atribuye al `bff`/`operacion`. Este carril aún NO aplica ese filtro de alcance: reenvía la lista de activos tal cual. Queda como pendiente anotado (el recorte por alcance del operador es trabajo separado); hoy el `bff` no lee rol ni alcance para esta ruta.

**Cambios:**
- `internal/agregacion/ports.go`: puerto `CatalogosPort` (método `ListarClientes(ctx, bearer, params)`) y campo `Catalogos` en `Deps`.
- `ports.go`: alias `CatalogosPort` y `PresupuestoCatalogos`.
- `adapters/http/catalogos.go` (nuevo): adaptador HTTP sobre el cliente generado de `catalogos`, con passthrough del Bearer (`conBearerCatalogos`) y la taxonomía de error de la plataforma.
- `internal/agregacion/handlers.go`: handler `ListarClientes` (reenvía el cursor, propaga el Bearer) y helper `aClientesBff` que traduce `catclient.PaginaClientes`→`bffclient.PaginaClientes` (espejo, paquetes generados distintos).
- `bff.go`: `Catalogos` en `Deps`/`New` y ruta `GET /api/v1/clientes` (protegida).
- `bff_test.go`: `listarClientes` entra en la lista de rutas montadas contra el contrato.
- `go.mod`/`go.sum`: pin de `api-client-go` a `v0.1.0-tracer.20`.

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
