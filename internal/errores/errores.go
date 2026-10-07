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
func DeStatus(op string, status int) error {
	switch status {
	case http.StatusUnauthorized:
		return platformerrors.NewNoAutorizado("bff: %s: no autorizado aguas abajo", op)
	case http.StatusForbidden:
		return platformerrors.NewNoAutorizado("bff: %s: prohibido aguas abajo", op)
	case http.StatusNotFound:
		return platformerrors.NewNoEncontrado("bff: %s: no encontrado aguas abajo", op)
	case http.StatusConflict:
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
