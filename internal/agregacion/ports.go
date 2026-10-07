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

	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
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

// OperacionPort es la arista a operacion: crear viaje, listar por alcance y
// obtener detalle. Sin transiciones internas ni asignacion (bff.md (f)).
type OperacionPort interface {
	// CrearViaje reenvia la creacion; el estado inicial lo fija el rol del token
	// en operacion (DEC-026), bff no decide.
	CrearViaje(ctx context.Context, body opclient.CrearViajeEntrada) (*opclient.Viaje, error)
	// ListarViajes lista por alcance; el tenant y el rol salen del token y el
	// alcance (unidad/cliente) se resuelve en servidor (DEC-053).
	ListarViajes(ctx context.Context, params opclient.ListarViajesParams) (*opclient.PaginaViajes, error)
	// ObtenerViaje lee el detalle para la composicion de hitos (ADR 27).
	ObtenerViaje(ctx context.Context, id opclient.Id) (*opclient.ViajeDetalle, error)
}

// Deps son los puertos que los handlers necesitan. La raiz los resuelve y New
// los inyecta.
type Deps struct {
	Identidad IdentidadPort
	Operacion OperacionPort
}
