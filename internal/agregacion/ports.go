// Package agregacion contiene los handlers HTTP de las operaciones de bff.yaml y
// las interfaces de los puertos de salida que esos handlers consumen. Los
// puertos se definen aqui, donde se usan ("accept interfaces", §6 de las buenas
// practicas de Go y regla 2 de 00), y el paquete raiz bff los re-exporta como
// alias para su superficie publica.
//
// Las interfaces estan tipadas SOLO con tipos de pf-pkg-contracts (regla 6 de
// 00): cada metodo corresponde a una operacion del cliente generado del destino.
package agregacion

import (
	"context"

	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
)

// IdentidadPort es la arista a identidad. bff reenvia credenciales y comandos de
// sesion; NUNCA verifica tokens (eso es local, con auth.Verifier). Incluye el
// snapshot de revocadas y la consulta de sesion para el fallback de ADR 26.
type IdentidadPort interface {
	// EmitirSesion reenvia credenciales a identidad.emitirSesion. bff no valida
	// credenciales (ruta publica).
	EmitirSesion(ctx context.Context, body idclient.CredencialesEntrada) (*idclient.SesionEmitida, error)
	// RefrescarSesion es el punto de reevaluacion de claims (ADR 26).
	RefrescarSesion(ctx context.Context, body idclient.RefrescoEntrada) (*idclient.SesionEmitida, error)
	// RevocarSesion revoca una sesion; identidad publica sesion.revocada.
	RevocarSesion(ctx context.Context, id idclient.Id) error
	// ListarSesionesRevocadas es el snapshot de reconstruccion de la proyeccion
	// de tombstones (ADR 26 §3).
	ListarSesionesRevocadas(ctx context.Context) (*idclient.PaginaRevocaciones, error)
	// ObtenerSesion es el fallback cuando el canal de revocacion no tiene frescura
	// (ADR 26). Nunca verifica el token: solo consulta estado de la sesion.
	ObtenerSesion(ctx context.Context, id idclient.Id) (*idclient.EstadoSesion, error)
}

// OperacionPort es la arista a operacion: crear viaje, listar por alcance,
// obtener detalle, leer bitacora y auditoria, y dejar el rastro de la descarga.
// Sin transiciones internas ni asignacion (bff.md (f)).
type OperacionPort interface {
	// CrearViaje reenvia la creacion; el estado inicial lo fija el rol del token
	// en operacion (DEC-026), bff no decide.
	CrearViaje(ctx context.Context, body opclient.CrearViajeEntrada) (*opclient.Viaje, error)
	// ListarViajes lista por alcance; el tenant y el rol salen del token y el
	// alcance (unidad/cliente) se resuelve en servidor (DEC-053).
	ListarViajes(ctx context.Context, params opclient.ListarViajesParams) (*opclient.PaginaViajes, error)
	// ObtenerViaje lee el detalle para la composicion de hitos (ADR 27) y para la
	// ficha tecnica del expediente de descarga (E6-T3).
	ObtenerViaje(ctx context.Context, id opclient.Id) (*opclient.ViajeDetalle, error)
	// ListarBitacora lee UNA pagina del historico de bitacora del viaje (E6-T3).
	// El cursor opaco viaja en params; la composicion de la descarga agota las
	// paginas (ADR 27: operacion es la fuente exacta, nunca reportes).
	ListarBitacora(ctx context.Context, id opclient.Id, params opclient.ListarBitacoraParams) (*opclient.PaginaBitacora, error)
	// ConsultarAuditoria lee UNA pagina de la auditoria relevante del viaje
	// (E6-T3). El cursor opaco viaja en params; la composicion agota las paginas.
	ConsultarAuditoria(ctx context.Context, id opclient.Id, params opclient.ConsultarAuditoriaParams) (*opclient.PaginaAuditoria, error)
	// RegistrarDescargaViaje pide a operacion que DEJE el rastro de auditoria de
	// una descarga exitosa (actor, viaje, momento; E1-T7). El rastro lo escribe
	// operacion en su transaccion, NO bff (que no tiene base de datos). Sin
	// cuerpo: el actor sale de los claims del token aguas abajo. Devuelve el
	// Viaje descargado, que bff descarta (la respuesta al cliente es el
	// expediente ya compuesto).
	RegistrarDescargaViaje(ctx context.Context, id opclient.Id) (*opclient.Viaje, error)
	// TransicionarViaje reenvia una transicion a operacion.transicionarViaje
	// (E9-T7). operacion es la unica autoridad: valida la arista, aplica el
	// aislamiento por tenant (cliente solo su viaje, DEC-112) y la facultad
	// (cliente solo CANCELADO/RECHAZADO_POR_CLIENTE, DEC-111). Devuelve el Viaje
	// plano; el handler recompone la vista de seguimiento con ObtenerViaje.
	TransicionarViaje(ctx context.Context, id opclient.Id, body opclient.TransicionEntrada) (*opclient.Viaje, error)
}

// MensajeriaPort es la arista a mensajeria: el portal cliente de emergencia
// (E7/tracer.17). Primera arista bff→mensajeria. Los métodos reciben el Bearer
// del usuario para propagarlo: mensajeria autoriza por los claims del token
// (ADR 21 §6), así que el token debe viajar aguas abajo.
type MensajeriaPort interface {
	// ListarEmergencias lista los casos del viaje; mensajeria proyecta por
	// audiencia (vista mínima para el cliente, ADR 21 §6).
	ListarEmergencias(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.PaginaEmergencias, error)
	// ActivarEmergencia activa un caso (idempotente por activacion_id).
	ActivarEmergencia(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.ActivarEmergenciaEntrada) (*msgclient.CasoEmergencia, error)
	// AgregarNotaEmergencia agrega una nota al caso (append-only).
	AgregarNotaEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.NotaEmergenciaEntrada) error

	// --- Atención interna del caso (panel de Reacción, E7/tracer.17) ---
	// Las cinco son acciones de escritura del operador interno. mensajeria
	// autoriza por los claims (PuedeAtenderEmergencia, ADR 21 §6); bff propaga
	// el Bearer y reenvía, no valida el rol.

	// ReclamarEmergencia toma el caso para el operador del token (concurrencia
	// optimista por version_esperada). Devuelve el reclamo abierto.
	ReclamarEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.ReclamarEmergenciaJSONRequestBody) (*msgclient.Reclamo, error)
	// LiberarReclamoEmergencia suelta el reclamo (version_esperada va como query).
	// La respuesta del servicio no lleva cuerpo; basta el éxito.
	LiberarReclamoEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID, reclamoID msgclient.Id, params msgclient.LiberarReclamoEmergenciaParams) error
	// TransferirEmergencia pasa el reclamo a otro responsable. Devuelve el reclamo
	// resultante.
	TransferirEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.TransferirEmergenciaJSONRequestBody) (*msgclient.Reclamo, error)
	// ResolverEmergencia resuelve el caso con una nota obligatoria. Devuelve el
	// caso actualizado.
	ResolverEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.ResolverEmergenciaJSONRequestBody) (*msgclient.CasoEmergencia, error)
	// CerrarEmergencia cierra el caso (sin cuerpo). Devuelve el caso cerrado.
	CerrarEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error)

	// --- Conversación del viaje para el portal cliente (E9-T8) -----------
	// La conversación se identifica por el viaje (DEC-029): el id de ruta de
	// mensajeria es el viaje_id. bff propaga el Bearer; mensajeria autoriza por
	// los claims y proyecta por audiencia (vista Cliente, ADR 21 §6).

	// ObtenerConversacion lee la conversación del viaje. mensajeria devuelve el
	// Conversacion completo; el bff recorta a la vista de cliente.
	ObtenerConversacion(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.Conversacion, error)
	// ListarMensajes lee una página de la historia, paginada por secuencia
	// (antes_de/despues_de/limite; catch-up con despues_de tras reconexión).
	ListarMensajes(ctx context.Context, bearer string, viajeID msgclient.Id, params msgclient.ListarMensajesParams) (*msgclient.PaginaMensajes, error)
	// EnviarMensaje publica un mensaje del cliente (idempotente por
	// client_message_id). Devuelve el Mensaje; 200 (reintento) o 201 (nuevo).
	EnviarMensaje(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.EnviarMensajeEntrada) (*msgclient.Mensaje, error)
}

// CatalogosPort es la arista a catalogos: el selector de clientes para crear un
// viaje por Operación (E8-T11). bff reenvía el cursor y propaga el Bearer;
// catalogos devuelve SOLO clientes activos (DEC-101 #1). El filtro de alcance del
// jefe_operativo (DEC-026 #3) NO lo aplica catalogos ni hoy el bff: queda
// pendiente (DEC-101 #2 lo atribuye a bff/operacion). La regla de puerta única
// impide al frontend llamar a catalogos directo: bff lo proyecta, mismo patrón
// que listarViajes.
type CatalogosPort interface {
	// ListarClientes lista una página de clientes activos. El cursor opaco viaja
	// en params; el filtro de solo-activos lo resuelve catalogos en servidor.
	ListarClientes(ctx context.Context, bearer string, params catclient.ListarClientesParams) (*catclient.PaginaClientes, error)
}

// Deps son los puertos que los handlers necesitan. La raiz los resuelve y New
// los inyecta.
type Deps struct {
	Identidad  IdentidadPort
	Operacion  OperacionPort
	Mensajeria MensajeriaPort
	Catalogos  CatalogosPort
}
