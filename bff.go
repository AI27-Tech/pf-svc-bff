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
	"fmt"
	"net/http"
	"time"

	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/AI27-Tech/pf-pkg-platform-go/bus"
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
	// Mensajeria es la arista a mensajeria: el portal cliente de emergencia
	// (E7/tracer.17). Primera arista bff→mensajeria; propaga el token del usuario.
	Mensajeria MensajeriaPort
	// Catalogos es la arista a catalogos: el selector de clientes para crear un
	// viaje por Operación (E8-T11); propaga el token del usuario.
	Catalogos CatalogosPort

	// Verificador verifica JWT localmente contra el JWKS del IdP interno
	// (auth.NewVerifier). Nunca viaja a identidad para verificar (ADR 26).
	Verificador auth.Authenticator
	// Broadcaster es el puerto de difusion del bus (ADR 29 §3). Si no es nil, bff
	// construye internamente su proyeccion de tombstones y arranca el consumidor
	// de sesion.revocada que la alimenta; el middleware consulta esa MISMA
	// proyeccion. La proyeccion es asunto interno de bff (no la construye la raiz):
	// asi el middleware y el worker comparten instancia por construccion, y la
	// raiz no necesita nombrar un tipo de internal/. Nil en desarrollo sin bus:
	// bff sirve con verificacion de firma pero sin consulta de tombstones.
	Broadcaster bus.Broadcaster
}

// Module es la instancia construida del modulo bff.
type Module struct {
	deps       Deps
	handlers   *agregacion.Handlers
	consumidor worker.Worker
	// revocacion es la proyeccion local de tombstones. La construye New cuando
	// hay Broadcaster; el middleware la consulta y el worker la alimenta. Nil si
	// no hay bus. Es *autorizacion.Proyeccion, un tipo de internal/: por eso bff
	// la crea y la retiene aqui, en vez de recibirla de la raiz.
	revocacion *autorizacion.Proyeccion
}

// New recibe los pools (vacios en bff) y los puertos ya resueltos. No lee
// entorno (00 regla 4): la raiz cablea todo. Si la raiz inyecta un Broadcaster,
// bff construye su proyeccion de tombstones y el consumidor de sesion.revocada
// que la alimenta.
func New(_ Pools, d Deps) (*Module, error) {
	h := agregacion.NewHandlers(agregacion.Deps{
		Identidad:  d.Identidad,
		Operacion:  d.Operacion,
		Mensajeria: d.Mensajeria,
		Catalogos:  d.Catalogos,
	})
	m := &Module{deps: d, handlers: h}

	// El consumidor solo se construye si hay bus. Sin el, bff sirve con
	// verificacion de firma pero sin tombstones (desarrollo).
	if d.Broadcaster != nil {
		// bff es dueño de su proyeccion: la crea aqui para que el middleware y el
		// worker compartan la MISMA instancia por construccion.
		proy := autorizacion.NuevaProyeccion()
		m.revocacion = proy

		// La misma SenalCanal la refresca el consumidor (con cada sobre del canal)
		// y la lee la proyeccion (para decidir si el canal esta obsoleto). Al
		// cablear la frescura, la proyeccion entra en la regla de ADR 26 §5:
		// canal obsoleto -> fallback a identidad -> fail-closed. El umbral de 15s
		// es normativo (ADR 26 §5), no configuracion de despliegue.
		senal := autorizacion.NuevaSenalCanal(time.Now)
		autoridad := autorizacion.NuevaAutoridad(d.Identidad)
		proy.ConFrescura(time.Now, senal, autoridad, umbralFrescura)

		cons, err := autorizacion.NuevoConsumidorRevocacion(d.Broadcaster, proy, senal)
		if err != nil {
			return nil, fmt.Errorf("construir consumidor de revocacion: %w", err)
		}
		// Envuelve el consumidor con la reconstrucción por snapshot (ADR 26 §4):
		// precarga las revocadas vigentes de identidad al arrancar y reporta
		// not-ready hasta converger. La suscripción arranca antes de la precarga
		// para no perder revocaciones durante la carga.
		snapshot := autorizacion.NuevoSnapshot(d.Identidad)
		m.consumidor = autorizacion.NuevoReconstructor(cons, snapshot, proy)
	}
	return m, nil
}

// umbralFrescura es la ventana tras la cual el canal de revocacion se considera
// obsoleto y las rutas protegidas caen al fallback durable con identidad
// (ADR 26 §5). Es normativo, no configurable por despliegue.
const umbralFrescura = 15 * time.Second

// Routes monta las rutas HTTP del modulo en el mux dado. Las rutas publicas
// (iniciar sesion) van sin middleware; las protegidas pasan por el verificador
// local y la consulta de tombstones (ADR 26). Una prueba compara las rutas
// montadas contra los operationId del contrato.
//
// El carril interno de emergencia vive bajo /api/v1/panel (E7/tracer.17): la
// misma ruta .../emergencias de /api/v1/viajes ya la ocupa el portal cliente, y
// un ServeMux no admite dos handlers para el mismo patrón, así que el panel usa
// su propio prefijo. No monta /tracking ni /panico (fuera de la version fijada).
func (m *Module) Routes(mux *http.ServeMux) {
	// Rutas publicas: bff no valida credenciales, solo reenvia a identidad.
	mux.HandleFunc("POST /api/v1/sesion", m.handlers.IniciarSesion)
	mux.HandleFunc("POST /api/v1/sesion/refresh", m.handlers.RefrescarSesion)

	// Rutas protegidas: verificacion local + consulta de tombstones antes de
	// aceptar (ADR 26). El middleware de plataforma inyecta las claims en el ctx.
	// La proyeccion (m.revocacion) es un *Proyeccion; si es nil (sin bus), se pasa
	// un RevocationChecker nil de interfaz (no un puntero nil envuelto), para que
	// el middleware omita la consulta de tombstones en vez de hacer panic.
	var rev auth.RevocationChecker
	if m.revocacion != nil {
		rev = m.revocacion
	}
	protegido := autorizacion.Middleware(m.deps.Verificador, rev)
	mux.Handle("DELETE /api/v1/sesion", protegido(http.HandlerFunc(m.handlers.CerrarSesion)))
	mux.Handle("POST /api/v1/viajes", protegido(http.HandlerFunc(m.handlers.CrearViaje)))
	mux.Handle("GET /api/v1/viajes", protegido(http.HandlerFunc(m.handlers.ListarViajes)))
	mux.Handle("GET /api/v1/viajes/{id}", protegido(http.HandlerFunc(m.handlers.SeguirViaje)))
	// Cancelar o rechazar el viaje desde el portal cliente (E9-T7, DEC-111). El
	// handler exige audiencia Cliente y reenvia a operacion, que es la unica
	// autoridad de transicion (aislamiento por tenant + facultad + motivo).
	mux.Handle("POST /api/v1/viajes/{id}/transiciones-cliente", protegido(http.HandlerFunc(m.handlers.TransicionarViajeCliente)))
	// Selector de clientes para crear un viaje por Operación (E8-T11): reenvío a
	// catalogos, solo clientes activos del alcance del jefe_operativo.
	mux.Handle("GET /api/v1/clientes", protegido(http.HandlerFunc(m.handlers.ListarClientes)))
	// Descarga del expediente de UN viaje (E6-T3, solo interno). El check de
	// audiencia vive en el handler, antes de componer nada; aqui solo se protege
	// con verificacion local + tombstones como las demas rutas.
	mux.Handle("GET /api/v1/viajes/{id}/descarga", protegido(http.HandlerFunc(m.handlers.DescargarViaje)))
	mux.Handle("GET /api/v1/viajes/{id}/emergencias", protegido(http.HandlerFunc(m.handlers.ListarEmergenciasCliente)))
	mux.Handle("POST /api/v1/viajes/{id}/emergencias", protegido(http.HandlerFunc(m.handlers.ActivarEmergenciaCliente)))
	mux.Handle("POST /api/v1/viajes/{id}/emergencias/{eid}/notas", protegido(http.HandlerFunc(m.handlers.AgregarNotaEmergenciaCliente)))

	// Conversación del viaje para el portal cliente (E9-T8, DEC-029): obtener la
	// conversación, listar la historia por secuencia y enviar. mensajeria autoriza
	// por claims; bff propaga el Bearer, exige audiencia Cliente y recorta la vista.
	mux.Handle("GET /api/v1/viajes/{id}/conversacion", protegido(http.HandlerFunc(m.handlers.ObtenerConversacionViaje)))
	mux.Handle("GET /api/v1/viajes/{id}/conversacion/mensajes", protegido(http.HandlerFunc(m.handlers.ListarMensajesViaje)))
	mux.Handle("POST /api/v1/viajes/{id}/conversacion/mensajes", protegido(http.HandlerFunc(m.handlers.EnviarMensajeViaje)))

	// Carril interno de emergencia (panel de Reacción, E7/tracer.17): listar con
	// vista completa y las cinco acciones de atención. mensajeria autoriza por
	// claims; bff propaga el Bearer.
	mux.Handle("GET /api/v1/panel/viajes/{id}/emergencias", protegido(http.HandlerFunc(m.handlers.ListarEmergenciasPanel)))
	mux.Handle("POST /api/v1/panel/viajes/{id}/emergencias/{eid}/reclamos", protegido(http.HandlerFunc(m.handlers.ReclamarEmergenciaPanel)))
	mux.Handle("DELETE /api/v1/panel/viajes/{id}/emergencias/{eid}/reclamos/{rid}", protegido(http.HandlerFunc(m.handlers.LiberarReclamoEmergenciaPanel)))
	mux.Handle("POST /api/v1/panel/viajes/{id}/emergencias/{eid}/transferencias", protegido(http.HandlerFunc(m.handlers.TransferirEmergenciaPanel)))
	mux.Handle("POST /api/v1/panel/viajes/{id}/emergencias/{eid}/resolucion", protegido(http.HandlerFunc(m.handlers.ResolverEmergenciaPanel)))
	mux.Handle("POST /api/v1/panel/viajes/{id}/emergencias/{eid}/cierre", protegido(http.HandlerFunc(m.handlers.CerrarEmergenciaPanel)))
}

// Workers devuelve los workers embebidos del modulo (ADR 29 §6). bff tiene uno:
// el consumidor de sesion.revocada, con su proyeccion local de tombstones,
// aunque comparta proceso con otros modulos en core. La raiz no comparte
// consumidor ni proyeccion entre modulos.
//
// Devuelve el consumidor solo si la raiz inyecto el Broadcaster y la proyeccion
// (ver New). Sin bus (desarrollo), la lista va vacia: bff sirve con verificacion
// de firma pero sin tombstones. La reconstruccion por snapshot y la readiness
// del canal (ADR 26) son fases siguientes.
func (m *Module) Workers() []worker.Worker {
	if m.consumidor == nil {
		return nil
	}
	return []worker.Worker{m.consumidor}
}
