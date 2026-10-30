// Package errores traduce fallos aguas abajo a la taxonomia de
// pf-pkg-platform-go/errors y al Error de bff.yaml. Regla de bff (bff.md (d)):
// un aguas-abajo caido es DEGRADADO o ROTO, NUNCA NO_ENCONTRADO. Una pieza
// ausente por degradacion viaja marcada, no como vacio: "un dato ausente por
// degradacion no es un dato ausente".
package errores

import (
	"context"
	"errors"
	"net/http"

	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
)

// DeStatus traduce un codigo HTTP de una respuesta aguas abajo a la taxonomia de
// la plataforma. op nombra la operacion para el contexto del error.
//
// Mapea tal cual los codigos de negocio (401/403/404/409/400), y trata 5xx y
// cualquier otro inesperado como ROTO: un fallo del propio aguas-abajo, no una
// ausencia de dato. El llamador decide si lo degrada (bff.md (d)).
//
// No conserva el codigo de maquina del cuerpo aguas abajo: para propagarlo (p.
// ej. el 409 de cierre con cierre_anticipado_requiere_incidencia), el adaptador
// usa DeRespuesta con el Codigo del Error tipado.
func DeStatus(op string, status int) error {
	return DeRespuesta(op, status, "")
}

// DeRespuesta es DeStatus con el codigo de maquina del cuerpo aguas abajo. Si
// codigo no esta vacio y el status es 409, el conflicto conserva ese codigo para
// que el cliente lo distinga (Ola C). Para los demas status el codigo se ignora:
// la categoria ya fija el codigo generico en el borde.
func DeRespuesta(op string, status int, codigo string) error {
	switch status {
	case http.StatusUnauthorized:
		return platformerrors.NewNoAutorizado("bff: %s: no autorizado aguas abajo", op)
	case http.StatusForbidden:
		// 403 de facultad/alcance aguas abajo (operacion). Se propaga como 403,
		// no como 401: la plataforma ya distingue Prohibido de NoAutorizado.
		return platformerrors.NewProhibido("bff: %s: prohibido aguas abajo", op)
	case http.StatusNotFound:
		return platformerrors.NewNoEncontrado("bff: %s: no encontrado aguas abajo", op)
	case http.StatusConflict:
		if codigo != "" {
			return platformerrors.NewConflictoCodigo(codigo, "bff: %s: conflicto aguas abajo", op)
		}
		return platformerrors.NewConflicto("bff: %s: conflicto aguas abajo", op)
	case http.StatusBadRequest:
		return platformerrors.NewValidacion("bff: %s: solicitud invalida aguas abajo", op)
	default:
		return platformerrors.NewRoto("bff: %s: respuesta inesperada aguas abajo (status %d)", op, status)
	}
}

// DeTransporte traduce un error de transporte (la llamada ni siquiera obtuvo
// respuesta: timeout, cancelacion, conexion caida) a DEGRADADO. Nunca es
// NO_ENCONTRADO: el aguas-abajo no respondio, no afirmo ausencia (bff.md (d)).
func DeTransporte(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return platformerrors.NewDegradadoErr(err, "bff: %s: presupuesto agotado aguas abajo", op)
	}
	return platformerrors.NewDegradadoErr(err, "bff: %s: fallo de transporte aguas abajo", op)
}
