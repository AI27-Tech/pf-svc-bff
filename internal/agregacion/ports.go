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
	repclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/reportes"
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

	// Gobierno de usuarios, alcance y tenants (tracer.40, E8-T19). Cada método
	// reenvía al operationId homónimo de identidad, que es la autoridad de rol y
	// de reglas: sus 403 y 409 llegan con su `codigo`. El actor sale de los
	// claims: el adaptador en proceso los encuentra en el ctx
	// (auth.ContextWithClaims del middleware) y el HTTP propaga el Bearer de
	// tokenctx. Mismas firmas que identidad.Consultas().Gobierno().
	ListarSujetos(ctx context.Context, params idclient.ListarSujetosParams) (*idclient.PaginaSujetos, error)
	AltaSujeto(ctx context.Context, body idclient.AltaSujetoEntrada) (*idclient.Sujeto, error)
	ObtenerSujeto(ctx context.Context, id idclient.Id) (*idclient.SujetoDetalle, error)
	EditarSujeto(ctx context.Context, id idclient.Id, body idclient.EdicionSujetoEntrada) (*idclient.SujetoDetalle, error)
	DesactivarSujeto(ctx context.Context, id idclient.Id) error
	ReactivarSujeto(ctx context.Context, id idclient.Id) error
	FijarRolSujeto(ctx context.Context, id idclient.Id, body idclient.AsignacionRolEntrada) error
	RevocarSesionesSujeto(ctx context.Context, id idclient.Id, body idclient.RevocacionSesionesEntrada) (*idclient.RevocacionSesionesResultado, error)
	ObtenerAlcanceSujeto(ctx context.Context, id idclient.Id) (*idclient.AlcanceSujeto, error)
	FijarUnidadesSujeto(ctx context.Context, id idclient.Id, body idclient.UnidadesSujetoEntrada) error
	FijarSubordinadosSujeto(ctx context.Context, id idclient.Id, body idclient.SubordinadosSujetoEntrada) error
	ListarTenants(ctx context.Context, params idclient.ListarTenantsParams) (*idclient.PaginaTenants, error)
	// CrearTenant es idempotente por id: creado es false si ya existía (200).
	CrearTenant(ctx context.Context, id idclient.Id, body idclient.TenantEntrada) (t *idclient.Tenant, creado bool, err error)
	DesactivarTenant(ctx context.Context, id idclient.Id) error
}

// OperacionPort es la arista a operacion: crear viaje, listar por alcance,
// obtener detalle, transicionar, leer bitacora y auditoria, dejar el rastro de
// la descarga y el flujo interno del panel (ETA, asignacion, bitacora,
// incidencias, estadias).
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

	// Flujo interno del panel (bff como única puerta pública). Cada uno reenvía
	// al operationId homónimo de operacion (DEC-105); operacion es la autoridad
	// de rol, estado y conflictos, bff solo exige audiencia Interna.

	// FijarEta reenvia la captura manual del ETA (ADR 20). Devuelve el Viaje
	// plano; el handler recompone el detalle interno con ObtenerViaje.
	FijarEta(ctx context.Context, id opclient.Id, body opclient.EtaEntrada) (*opclient.Viaje, error)
	// AsignarRecursos reenvia la reserva atomica de la escolta (ADR 31).
	AsignarRecursos(ctx context.Context, id opclient.Id, body opclient.AsignacionEntrada) (*opclient.Asignacion, error)
	// RegistrarEntradaBitacora reenvia una entrada append-only; el autor sale del
	// token aguas abajo.
	RegistrarEntradaBitacora(ctx context.Context, id opclient.Id, body opclient.EntradaBitacoraEntrada) (*opclient.EntradaBitacora, error)
	// AbrirIncidencia y ResolverIncidencia reenvian la condicion INCIDENCIA; el rol
	// del token decide la facultad en operacion.
	AbrirIncidencia(ctx context.Context, id opclient.Id, body opclient.IncidenciaEntrada) (*opclient.Incidencia, error)
	ResolverIncidencia(ctx context.Context, id, iid opclient.Id, body opclient.ResolucionIncidenciaEntrada) (*opclient.Incidencia, error)
	// ListarEstadias, AbrirEstadia y CerrarEstadia reenvian las estadias del viaje.
	ListarEstadias(ctx context.Context, id opclient.Id, params opclient.ListarEstadiasParams) (*opclient.PaginaEstadias, error)
	AbrirEstadia(ctx context.Context, id opclient.Id, body opclient.EstadiaEntrada) (*opclient.Estadia, error)
	CerrarEstadia(ctx context.Context, id, eid opclient.Id) (*opclient.Estadia, error)

	// Evidencia del viaje (E6-HU-027 / E8-T17). Reenvían al operationId homónimo
	// de operacion. Los bytes NO pasan por bff: la subida y la descarga usan la
	// URL de vida corta del almacenamiento de objetos, y la auditoría de cada
	// acceso la escribe operacion.
	ListarAdjuntos(ctx context.Context, id opclient.Id) (*opclient.PaginaAdjuntos, error)
	CrearIntencionSubidaAdjunto(ctx context.Context, id opclient.Id, body opclient.IntencionSubidaAdjuntoEntrada) (*opclient.IntencionSubidaAdjunto, error)
	DescargarAdjunto(ctx context.Context, id, aid opclient.Id) (*opclient.DescargaAdjunto, error)

	// ObtenerAsignacion lee la escolta vigente del viaje (unidades del roster con
	// vehículo y custodios por id) para el detalle de Reacción (E8-T18).
	ObtenerAsignacion(ctx context.Context, id opclient.Id) (*opclient.EscoltaVigente, error)

	// Flujo interno, grupo 2 (E8-T9/T10/T12/T13). Mismo patrón: reenvío al
	// operationId homónimo; operacion decide rol y estado.

	// ListarUnidades lee UNA página del roster para asignar la escolta.
	ListarUnidades(ctx context.Context, params opclient.ListarUnidadesParams) (*opclient.PaginaUnidades, error)
	// CrearUnidad, ObtenerUnidad y ModificarUnidad arman el roster en /roster
	// (E8-T21, DEC-142). operacion decide el rol (jefe_operativo,
	// administrador) y las reglas; sus 409 llegan con codigo
	// (vehiculo_en_otra_unidad, custodio_en_otra_unidad,
	// unidad_con_reserva_vigente).
	CrearUnidad(ctx context.Context, body opclient.UnidadEntrada) (*opclient.Unidad, error)
	ObtenerUnidad(ctx context.Context, id opclient.Id) (*opclient.Unidad, error)
	ModificarUnidad(ctx context.Context, id opclient.Id, body opclient.UnidadModificacion) (*opclient.Unidad, error)
	// CompletarFechaHora fija fecha_hora_servicio antes de aprobar. Devuelve el
	// Viaje plano; el handler recompone el detalle interno con ObtenerViaje.
	CompletarFechaHora(ctx context.Context, id opclient.Id, body opclient.FechaHoraEntrada) (*opclient.Viaje, error)
	// ListarIncidencias lee UNA página del histórico de incidencias del viaje.
	ListarIncidencias(ctx context.Context, id opclient.Id, params opclient.ListarIncidenciasParams) (*opclient.PaginaIncidencias, error)
	// ObtenerFicha y GuardarFicha leen y editan por partes la ficha técnica
	// (DEC-104). En GuardarFicha, Tracto nil no toca el tracto y &Tracto{} lo
	// borra (el handler traduce `"tracto": null` a &Tracto{}).
	ObtenerFicha(ctx context.Context, id opclient.Id) (*opclient.FichaTecnica, error)
	GuardarFicha(ctx context.Context, id opclient.Id, body opclient.FichaTecnicaEntrada) (*opclient.FichaTecnica, error)
}

// ReportesPort es la arista a reportes: el dashboard de monitoreo (E6-HU-025 /
// E8-T17). Cada método reenvía al operationId homónimo y devuelve el tipo del
// cliente de reportes sin recortar: proyeccion_al y stale deben llegar al panel
// (DEC-033). reportes autoriza la audiencia por los claims; el Bearer del
// usuario viaja en el ctx (tokenctx).
type ReportesPort interface {
	ReporteViajes(ctx context.Context, params repclient.ReporteViajesParams) (*repclient.ReporteViajes, error)
	ReporteEstados(ctx context.Context, params repclient.ReporteEstadosParams) (*repclient.ReporteEstados, error)
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
	// idempotente es true en la reentrega de la misma activación (200), para que
	// el handler responda 200 y no 201.
	ActivarEmergencia(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.ActivarEmergenciaEntrada) (caso *msgclient.CasoEmergencia, idempotente bool, err error)
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

	// --- Bandeja de Reacción y caso por su id (tracer.34, DEC-121) -------
	// Mismos operationId que mensajeria. mensajeria autoriza por rol (DEC-130,
	// DEC-131); bff propaga el Bearer y reenvía, salvo ObtenerEmergencia, cuyo
	// caso usa el handler para componer el detalle con el viaje.
	ListarBandejaEmergencias(ctx context.Context, bearer string, params msgclient.ListarBandejaEmergenciasParams) (*msgclient.PaginaBandejaEmergencias, error)
	ObtenerEmergencia(ctx context.Context, bearer string, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error)
	ListarNotasEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, params msgclient.ListarNotasEmergenciaPorIdParams) (*msgclient.PaginaNotasEmergencia, error)
	ReclamarEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.VersionEsperadaEntrada) (*msgclient.Reclamo, error)
	LiberarReclamoEmergenciaPorId(ctx context.Context, bearer string, emergenciaID, reclamoID msgclient.Id, params msgclient.LiberarReclamoEmergenciaPorIdParams) error
	TransferirEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.TransferenciaEntrada) (*msgclient.Reclamo, error)
	ResolverEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.ResolucionEmergenciaEntrada) (*msgclient.EmergenciaReaccion, error)
	CerrarEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error)

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

	// --- Bandeja y conversación interna del pool (E8-T15) ----------------
	// Superficie interna del pool de Ejecutivos (reclamo, transferencia,
	// atención, recibos). Misma mecánica: bff propaga el Bearer y mensajeria
	// autoriza por los claims (membresía efectiva en el pool, no solo el rol).
	// A diferencia de la vista de cliente, el bff NO recorta la respuesta: la
	// audiencia interna ve el Conversacion/Mensaje completos (DEC-006 invertido).
	// ObtenerConversacion, ListarMensajes y EnviarMensaje se reutilizan de
	// arriba: el adaptador ya devuelve el tipo completo y el recorte vive en el
	// handler de cliente, no en el puerto.

	// ListarTrabajoBandeja lista el trabajo de la bandeja (bootstrap sin
	// desde_revision, catch-up con él). El bff resuelve la bandeja del sujeto
	// desde los claims (cola única, DEC-103); por eso NO recibe un id de bandeja.
	ListarTrabajoBandeja(ctx context.Context, bearer string, params msgclient.ListarTrabajoMiBandejaParams) (*msgclient.TrabajoBandeja, error)
	// ReclamarConversacion toma la conversación para el sujeto del token
	// (concurrencia optimista por version_esperada). Devuelve el reclamo abierto.
	ReclamarConversacion(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.VersionEsperadaEntrada) (*msgclient.Reclamo, error)
	// LiberarReclamo suelta el reclamo (version_esperada va como query). Sin cuerpo.
	LiberarReclamo(ctx context.Context, bearer string, conversacionID, reclamoID msgclient.Id, params msgclient.LiberarReclamoParams) error
	// TransferirConversacion pasa la responsabilidad a otro miembro del pool.
	// Devuelve el reclamo del destino.
	TransferirConversacion(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.TransferenciaEntrada) (*msgclient.Reclamo, error)
	// MarcarAtendida marca la conversación como atendida por el responsable.
	// Sin cuerpo de respuesta; basta el éxito.
	MarcarAtendida(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.VersionEsperadaEntrada) error
	// RegistrarReciboEntregado avanza el cursor de entrega del llamante. Sin cuerpo.
	RegistrarReciboEntregado(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.ReciboEntrada) error
	// RegistrarReciboLeido avanza el cursor de lectura del llamante. Sin cuerpo.
	RegistrarReciboLeido(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.ReciboEntrada) error
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
	// ListarTiposServicio lista una página del catálogo tipos_servicio para el
	// selector del wizard de solicitud del cliente (E9-T4). El bff fija el tipo
	// (el cliente no elige familia) y filtra a los activo al recortar.
	ListarTiposServicio(ctx context.Context, bearer string, params catclient.ListarValoresCatalogoParams) (*catclient.PaginaValoresCatalogo, error)

	// Lecturas por id para el detalle de Reacción (E8-T18): nombre del cliente,
	// económico y placas del vehículo, nombre y teléfono del custodio. Versión
	// vigente (sin a_fecha).
	ObtenerCliente(ctx context.Context, bearer string, id catclient.Id) (*catclient.Cliente, error)
	ObtenerVehiculo(ctx context.Context, bearer string, id catclient.Id) (*catclient.Vehiculo, error)
	ObtenerCustodio(ctx context.Context, bearer string, id catclient.Id) (*catclient.Custodio, error)

	// ListarGps lista una página de dispositivos GPS activos para el selector de
	// la ficha técnica (E8-T12, DEC-104 #5). El filtro lo resuelve catalogos.
	ListarGps(ctx context.Context, bearer string, params catclient.ListarGpsParams) (*catclient.PaginaGps, error)

	// Vehículos y custodios de /catalogos (adelanto de E8-T16, DEC-142 #6). La
	// lectura por id reusa ObtenerVehiculo y ObtenerCustodio. catalogos no
	// revisa rol: la guarda (leer jefe_operativo y administrador, escribir solo
	// administrador) la aplica el handler antes de llamar.
	ListarVehiculos(ctx context.Context, bearer string, params catclient.ListarVehiculosParams) (*catclient.PaginaVehiculos, error)
	CrearVehiculo(ctx context.Context, bearer string, body catclient.VehiculoEntrada) (*catclient.Vehiculo, error)
	ActualizarVehiculo(ctx context.Context, bearer string, id catclient.Id, body catclient.VehiculoCambios) (*catclient.Vehiculo, error)
	ListarCustodios(ctx context.Context, bearer string, params catclient.ListarCustodiosParams) (*catclient.PaginaCustodios, error)
	CrearCustodio(ctx context.Context, bearer string, body catclient.CustodioEntrada) (*catclient.Custodio, error)
	ActualizarCustodio(ctx context.Context, bearer string, id catclient.Id, body catclient.CustodioCambios) (*catclient.Custodio, error)

	// Expediente de la organización cliente (tracer.40, DEC-144 #2). catalogos
	// no revisa rol ni la terminalidad de la organización: las dos guardas son
	// del bff. CrearCliente es idempotente por id: creado es false si ya existía
	// (200). La lectura por lote de clientes es ListarClientes con params.Ids.
	CrearCliente(ctx context.Context, bearer string, id catclient.Id, body catclient.ClienteEntrada) (cl *catclient.Cliente, creado bool, err error)
	ActualizarCliente(ctx context.Context, bearer string, id catclient.Id, body catclient.ClienteCambios) (*catclient.Cliente, error)
}

// Deps son los puertos que los handlers necesitan. La raiz los resuelve y New
// los inyecta.
type Deps struct {
	Identidad  IdentidadPort
	Operacion  OperacionPort
	Mensajeria MensajeriaPort
	Catalogos  CatalogosPort
	Reportes   ReportesPort
}
