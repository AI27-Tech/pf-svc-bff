package agregacion

import (
	"encoding/json"
	"net/http"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/tracking"
	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"
)

// Handlers implementa las seis operaciones de la fase (a) de bff.yaml. Cada
// handler usa SOLO tipos generados de pf-pkg-contracts para su cuerpo de
// entrada/salida (regla 6 de 00): bff no define formas de wire a mano.
//
// bff agrega, autoriza y adapta; no guarda (bff.md (a)). Ningun handler orquesta
// una escritura multi-servicio (bff.md (h) R5).
type Handlers struct {
	deps Deps
}

// NewHandlers construye los handlers con los puertos ya resueltos.
func NewHandlers(d Deps) *Handlers {
	return &Handlers{deps: d}
}

// IniciarSesion delega en identidad.emitirSesion. Ruta publica: bff NO valida
// credenciales, solo reenvia (bff.yaml).
func (h *Handlers) IniciarSesion(w http.ResponseWriter, r *http.Request) {
	var cuerpo bffclient.CredencialesEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	// El contrato de bff y el de identidad comparten la forma de credenciales;
	// se traduce explicitamente para no acoplar los dos paquetes generados.
	sesion, err := h.deps.Identidad.EmitirSesion(r.Context(), idclient.CredencialesEntrada{
		Usuario:    cuerpo.Usuario,
		Contrasena: cuerpo.Contrasena,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, aSesionBff(sesion))
}

// RefrescarSesion delega en identidad.refrescarSesion. Es el punto de
// reevaluacion de claims (ADR 26). Ruta publica (el refresh token es la prueba).
func (h *Handlers) RefrescarSesion(w http.ResponseWriter, r *http.Request) {
	var cuerpo bffclient.RefrescoEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	sesion, err := h.deps.Identidad.RefrescarSesion(r.Context(), idclient.RefrescoEntrada{
		RefreshToken: cuerpo.RefreshToken,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aSesionBff(sesion))
}

// CerrarSesion delega en identidad.revocarSesion. Ruta protegida: el sesion_id a
// revocar sale de las claims verificadas, nunca de un parametro (P10). identidad
// publica sesion.revocada.
func (h *Handlers) CerrarSesion(w http.ResponseWriter, r *http.Request) {
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: cerrarSesion: sin claims"))
		return
	}
	id, err := uuidDe(claims.SesionID)
	if err != nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: cerrarSesion: sesion_id invalido en el token"))
		return
	}
	if err := h.deps.Identidad.RevocarSesion(r.Context(), id); err != nil {
		escribirError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CrearViaje delega en operacion.crearViaje. El estado inicial lo fija el rol del
// token en operacion (DEC-026); bff reenvia, no decide. Ruta protegida.
func (h *Handlers) CrearViaje(w http.ResponseWriter, r *http.Request) {
	var cuerpo bffclient.CrearViajeEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	viaje, err := h.deps.Operacion.CrearViaje(r.Context(), aCrearViajeOperacion(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, viaje)
}

// ListarViajes delega en operacion.listarViajes. El tenant y el rol salen del
// token (P9, P10); el alcance por unidad/cliente se resuelve en servidor, no
// viaja en el token (DEC-053). Ruta protegida.
func (h *Handlers) ListarViajes(w http.ResponseWriter, r *http.Request) {
	// En fase (a) solo se reenvia el cursor; los filtros de alcance los aplica
	// operacion a partir del contexto autenticado que propaga el adaptador.
	var params opclient.ListarViajesParams
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		c := opclient.Cursor(cursor)
		params.Cursor = &c
	}
	pagina, err := h.deps.Operacion.ListarViajes(r.Context(), params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// SeguirViaje delega en operacion.obtenerViaje y compone los hitos V1 por
// polling (ADR 27). El cliente recibe el hito traducido, nunca el token tecnico
// (E2-T2). Ruta protegida.
func (h *Handlers) SeguirViaje(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: seguirViaje: id invalido"))
		return
	}
	detalle, err := h.deps.Operacion.ObtenerViaje(r.Context(), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	// Composicion pura de hitos (E2-T2/E2-T3). Traduce estado tecnico -> hito y
	// colapsa el bucle multi-parada; el cliente nunca ve el token tecnico.
	escribirJSON(w, http.StatusOK, tracking.Componer(*detalle))
}

// aSesionBff traduce la SesionEmitida de identidad a la Sesion de bff. Ambas
// comparten forma, pero son tipos de paquetes generados distintos; la traduccion
// es explicita para no acoplarlos.
func aSesionBff(s *idclient.SesionEmitida) bffclient.Sesion {
	if s == nil {
		return bffclient.Sesion{}
	}
	return bffclient.Sesion{
		SesionId:     s.SesionId,
		AccessToken:  s.AccessToken,
		ExpiraEn:     s.ExpiraEn,
		RefreshToken: s.RefreshToken,
	}
}

// aCrearViajeOperacion traduce la entrada de bff a la de operacion. bff reenvia
// origen, destinos y datos opcionales; NO fija asignacion (bff.md (f)).
func aCrearViajeOperacion(e bffclient.CrearViajeEntrada) opclient.CrearViajeEntrada {
	destinos := make([]opclient.Destino, 0, len(e.Destinos))
	for _, d := range e.Destinos {
		destinos = append(destinos, opclient.Destino{
			Nombre:      d.Nombre,
			NumeroEnvio: d.NumeroEnvio,
		})
	}
	return opclient.CrearViajeEntrada{
		ClienteId:         e.ClienteId,
		Origen:            e.Origen,
		Destinos:          destinos,
		FechaHoraServicio: e.FechaHoraServicio,
		TipoServicio:      e.TipoServicio,
	}
}

// uuidDe parsea un UUID de dominio (Id del contrato = openapi_types.UUID, que a
// su vez es un alias de uuid.UUID de google/uuid).
func uuidDe(s string) (openapitypes.UUID, error) {
	return uuid.Parse(s)
}

// decodificar lee el cuerpo JSON de la peticion en v, traduciendo un cuerpo
// invalido a un error de validacion de la plataforma.
func decodificar(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return platformerrors.NewValidacion("bff: cuerpo de la peticion invalido: %v", err)
	}
	return nil
}

// escribirJSON serializa v como respuesta JSON con el status dado.
func escribirJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// escribirError traduce un error de la taxonomia de la plataforma al Error de
// bff.yaml y al status HTTP correspondiente. Reusa el mapeo de la plataforma
// (Category.HTTPStatus y Category.String) en vez de duplicarlo. Un aguas-abajo
// caido es degradado o roto, nunca no encontrado (bff.md (d)).
func escribirError(w http.ResponseWriter, err error) {
	cat := platformerrors.AsCategory(err)
	escribirJSON(w, cat.HTTPStatus(), bffclient.Error{
		Codigo:  cat.String(),
		Mensaje: err.Error(),
	})
}
