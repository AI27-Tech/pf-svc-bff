// Package bff es la capa de borde de AI27: la unica superficie HTTP de comandos
// y consultas que el front-end consume. Agrega, autoriza y adapta; NO guarda
// nada (bff.md (a), (e)). No es dueno de ningun dato: sin esquema, sin tabla,
// sin migracion, sin rol de BD.
//
// Capa de borde. Vive en el deployable `core` (ADR 17; 00, tabla de topologia).
// En produccion `core` lo compone y le inyecta adaptadores EN PROCESO para
// operacion, catalogos, identidad y reportes; mensajeria va siempre por HTTP.
// `cmd/api` de este repo arranca standalone, siempre con adaptadores HTTP
// (regla 3 de 00, I5); produccion no lo usa. Replicas, drenado y cadencia de
// liberacion son del proceso `core`, no de este modulo (bff.md (k), ADR 17).
//
// Aristas salientes: operacion, catalogos, identidad, reportes, mensajeria (00,
// tabla de servicios). Ningun servicio llama a bff: sin aristas entrantes, por
// eso NO expone Consultas() (00: solo existe si el modulo es destino de una
// arista).
//
// Scaffolding inicial: solo la fase (a) de bff.yaml (sesion y viajes) y la
// composicion pura de hitos (E2-T2/E2-T3, ADR 27). Los puertos de catalogos,
// reportes y mensajeria (fase b) se agregan cuando su superficie exista en la
// version fijada. Varias piezas estan bloqueadas por brechas de contrato (ver
// README): no se montan hasta que el contrato las exponga.
package bff

import (
	"net/http"

	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/AI27-Tech/pf-pkg-platform-go/worker"
	"github.com/AI27-Tech/pf-svc-bff/internal/agregacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
)

// Pools agrupa los pools por rol que un modulo de servicio recibiria de la raiz
// (00 regla 4). bff NO es dueno de ningun dato: no abre ni recibe conexion a
// Postgres. Se conserva como struct SIN campos solo para preservar la firma
// comun de `New(Pools, Deps)` de 00; permanece vacio a proposito (bff.md (e)).
type Pools struct{}

// Deps agrupa los puertos de salida y las dependencias de plataforma que la raiz
// resuelve por el consumidor (00 regla 2; §A.5 R2). En fase (a) solo se cablean
// identidad y operacion; catalogos, reportes y mensajeria se agregan en fase (b).
type Deps struct {
	// Identidad es la arista a identidad: emitir, refrescar y revocar sesion, mas
	// el snapshot de revocadas y la consulta de sesion para el fallback de ADR 26.
	// bff NUNCA verifica tokens contra identidad (eso es local, con auth.Verifier).
	Identidad IdentidadPort
	// Operacion es la arista a operacion: crear viaje, listar por alcance y obtener
	// detalle. Sin transiciones internas ni asignacion (bff.md (f)).
	Operacion OperacionPort

	// Verificador verifica JWT localmente contra el JWKS del IdP interno
	// (auth.NewVerifier). Nunca viaja a identidad para verificar (ADR 26).
	Verificador auth.Authenticator
	// Revocacion consulta la proyeccion local de tombstones antes de aceptar una
	// ruta protegida (ADR 26 §3). La alimenta el worker de sesion.revocada.
	Revocacion auth.RevocationChecker
}

// Module es la instancia construida del modulo bff.
type Module struct {
	deps     Deps
	handlers *agregacion.Handlers
}

// New recibe los pools (vacios en bff) y los puertos ya resueltos. No lee
// entorno (00 regla 4): la raiz cablea todo.
func New(_ Pools, d Deps) (*Module, error) {
	h := agregacion.NewHandlers(agregacion.Deps{
		Identidad: d.Identidad,
		Operacion: d.Operacion,
	})
	return &Module{deps: d, handlers: h}, nil
}

// Routes monta las rutas HTTP del modulo en el mux dado. Monta exactamente las
// seis operaciones de la fase (a) de bff.yaml. Las rutas publicas (iniciar
// sesion) van sin middleware; las protegidas pasan por el verificador local y la
// consulta de tombstones (ADR 26). Una prueba compara las rutas montadas contra
// los operationId del contrato.
//
// NO monta /api/v1/panel (no existe en el contrato, brecha reportada),
// /tracking, /panico ni ninguna ruta de la fase (b) (catalogos, reportes,
// mensajeria): sus puertos y superficie no existen todavia en la version fijada.
func (m *Module) Routes(mux *http.ServeMux) {
	// Rutas publicas: bff no valida credenciales, solo reenvia a identidad.
	mux.HandleFunc("POST /api/v1/sesion", m.handlers.IniciarSesion)
	mux.HandleFunc("POST /api/v1/sesion/refresh", m.handlers.RefrescarSesion)

	// Rutas protegidas: verificacion local + consulta de tombstones antes de
	// aceptar (ADR 26). El middleware de plataforma inyecta las claims en el ctx.
	protegido := autorizacion.Middleware(m.deps.Verificador, m.deps.Revocacion)
	mux.Handle("DELETE /api/v1/sesion", protegido(http.HandlerFunc(m.handlers.CerrarSesion)))
	mux.Handle("POST /api/v1/viajes", protegido(http.HandlerFunc(m.handlers.CrearViaje)))
	mux.Handle("GET /api/v1/viajes", protegido(http.HandlerFunc(m.handlers.ListarViajes)))
	mux.Handle("GET /api/v1/viajes/{id}", protegido(http.HandlerFunc(m.handlers.SeguirViaje)))
}

// Workers devuelve los workers embebidos del modulo (ADR 29 §6). bff tiene uno:
// el consumidor de sesion.revocada, con su propia suscripcion durable en el
// broker V1 (DEC-057), su proyeccion local de tombstones, su snapshot de
// reconstruccion y su readiness, aunque comparta proceso con otros modulos en
// core. La raiz no comparte consumidor ni proyeccion entre modulos.
//
// Scaffolding: hoy devuelve una lista vacia. El consumidor se cablea cuando la
// raiz inyecte el adaptador de bus (bus/memory en desarrollo; broker real en
// espera de ADR 32) y la suscripcion durable propia exista. El codigo del
// consumidor y su proyeccion viven en internal/autorizacion.
func (m *Module) Workers() []worker.Worker {
	return nil
}
