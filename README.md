# pf-svc-bff

Capa de **borde** de AI27: la única superficie HTTP de comandos y consultas que
el front-end consume. `bff` **agrega, autoriza y adapta; no guarda nada**
(`docs/arquitectura/services/bff.md` (a), (e)). No es dueño de ningún dato: sin
esquema, sin tabla, sin migración, sin rol de BD.

Vive dentro del deployable `core` (ADR 17). En producción `core` lo compone y le
inyecta adaptadores **en proceso** para `operacion`, `catalogos`, `identidad` y
`reportes`; `mensajeria` va siempre por HTTP. Este repo también arranca
**standalone** (`cmd/api`, invariante I5) con adaptadores HTTP, solo para
levantar y probar el módulo aislado; producción no usa ese arranque.

## Alcance de este scaffold (fase a)

Solo la fase (a) de `bff.yaml` y la composición pura de hitos (ADR 27). Las seis
operaciones montadas:

| `operationId` | Ruta | Delega en | Acceso |
|---|---|---|---|
| `iniciarSesion` | `POST /api/v1/sesion` | `identidad.emitirSesion` | Pública |
| `cerrarSesion` | `DELETE /api/v1/sesion` | `identidad.revocarSesion` | Protegida |
| `refrescarSesion` | `POST /api/v1/sesion/refresh` | `identidad.refrescarSesion` | Pública |
| `crearViaje` | `POST /api/v1/viajes` | `operacion.crearViaje` | Protegida |
| `listarViajes` | `GET /api/v1/viajes` | `operacion.listarViajes` | Protegida |
| `seguirViaje` | `GET /api/v1/viajes/{id}` | `operacion.obtenerViaje` | Protegida |

### Composición de hitos V1 (`internal/tracking`)

`seguirViaje` es la lectura inicial y periódica del seguimiento del cliente (E2,
ADR 27). `bff` es su único compositor: traduce el estado técnico a un hito de
negocio (E2-T2), colapsando el bucle multi-parada (`EN_TRANSITO` → `EN_DESTINO`
→ `EN_DESCARGA`) en un solo "En camino". El cliente **nunca** recibe el token
técnico. Es función pura, con pruebas por estado. Sin SSE, sin posición, sin ETA
automática (ADR 20, 27). La cadencia de polling es configuración del cliente, no
contrato.

### Autorización y revocación (ADR 26)

Verificación de JWT **local**, con `pf-pkg-platform-go/auth` contra el JWKS del
IdP interno; `bff` nunca viaja a `identidad` para verificar. La `Audiencia` de
dominio (`Cliente` | `Interno`) se lee de `auth.Claims.Audiencia` (DEC-054),
nunca de un parámetro (P10); `auth.Claims.Audience` es el claim técnico `aud` y
**no autoriza**. Las rutas protegidas consultan la proyección local de
tombstones (`internal/autorizacion`) antes de aceptar, y **fallan cerrado** sin
señal utilizable. El consumidor de `sesion.revocada` será un `worker.Worker` con
suscripción durable propia (DEC-057); hoy la lista de `Workers()` está vacía
hasta que la raíz inyecte el bus.

## Presupuestos por dependencia

El agregador no degrada una vista ajena: cada arista tiene su propio presupuesto
y pool (`bff.md` (i)). Los **valores concretos no están en las fuentes y no se
inventan**: quedan como configuración (`PresupuestoOperacion`,
`PresupuestoIdentidad` son marcadores de posición que la raíz sobrescribe).

## Brechas de contrato reportadas (no resueltas)

Siguiendo la regla 6 de `00-indice.md` (contrato ausente → se detiene y se
reporta, no se inventa):

1. **`stale`** — Contradicción de fuentes. E2-T2 y ADR 20 dicen que `bff`
   calcula `stale` y que `operacion` **no** lo expone; pero `operacion.yaml`
   (`Eta.obsoleto`) sí lo trae calculado. El scaffold **reenvía** el valor del
   contrato sin recalcular, para no inventar una regla. La elección ("bff
   calcula" vs "operacion calcula") la decide Product. Ver
   `internal/tracking/tracking.go` (`Componer`).
2. **Incidencia curada (E2-T3)** — `operacion.ViajeDetalle.condiciones_activas`
   solo trae `tipo` (`INCIDENCIA`/`SIN_SENAL`) y `abierta_en`; no hay tipo
   interno, estado ni impacto que mapear a las cinco categorías curadas, y
   `operacion.yaml` no tiene lectura de incidencias (solo abrir/resolver). Sin
   esa superficie, `Incidencias` viaja **vacío** (no nulo). No se inventa el
   mapeo (DEC-006).
3. **`catalogo.actualizado`** — Contradicción entre fuentes: `INVENTARIO.md`
   lista a `operacion` como **único** consumidor, mientras `bff.md` (g) dice que
   `bff` lo consume. A resolver en el contrato, no aquí. (Fase b; sin puerto
   `catalogos` todavía.)
4. **`GET /api/v1/panel`** — No existe en el contrato (`PLAN-EJECUCION-V1.md`
   §9). El portal cliente lo consume por un mock; la decisión está diferida.
   **No se implementa.**
5. **Forma de sesión de los frontends** — Los portales asumen una forma distinta
   al contrato (PLAN §9). `bff` implementa **el contrato**; la corrección de los
   portales es `E8-T3`. No se adapta `bff` a lo que asumen los frontends.

## Fuera de alcance

Fase (b): puertos `catalogos`, `reportes`, `mensajeria` y sus rutas. V2:
`/tracking`, `POST /panico`, `procesamiento`, SSE. El stream de `realtime` **no
atraviesa `bff`**. El stub de auth de desarrollo es de `core`, no de este repo.

## Verificación

```sh
export GOPRIVATE='github.com/AI27-Tech/*'
gofmt -l .
go vet ./...
go build ./...
go test ./...
GOFLAGS=-mod=readonly go build ./...
go build ./cmd/api
go run github.com/AI27-Tech/pf-pkg-platform-go/tools/arch-test@v0.1.1 .
```

## Estructura

```
pf-svc-bff/
├── bff.go                 # New / Routes / Workers (sin Consultas: bff no tiene aristas entrantes)
├── ports.go               # IdentidadPort, OperacionPort (alias) + presupuestos
├── cmd/api/main.go        # arranque standalone (I5): adaptadores HTTP, sin BD
├── adapters/http/         # adaptadores HTTP sobre los clientes generados
│   ├── identidad.go
│   └── operacion.go
└── internal/
    ├── agregacion/        # handlers de las seis operaciones
    ├── autorizacion/      # verificación local + tombstones (ADR 26)
    ├── tracking/          # hitos V1 (E2-T2/E2-T3): función pura, con pruebas
    └── errores/           # adaptación a pf-pkg-platform-go/errors
```
