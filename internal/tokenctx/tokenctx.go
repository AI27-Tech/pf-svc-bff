// Package tokenctx lleva el Bearer del usuario en el contexto de la petición,
// para que un adaptador saliente lo propague aguas abajo sin acoplar la firma
// del puerto al transporte.
//
// Por qué el contexto y no un parámetro del puerto: la arista bff→operacion (y
// bff→identidad) tiene DOS adaptadores del mismo puerto. El HTTP (standalone,
// I5) cruza un proceso real y DEBE reenviar el token como header. El EN PROCESO
// de core NO cruza proceso: los claims ya viajan en el ctx de la petición y
// operacion los lee con auth.ClaimsFromContext; no hay token que re-serializar.
// Un parámetro `bearer string` en el puerto obligaría al adaptador en proceso a
// aceptar un dato que no usa. El contexto deja que cada adaptador tome solo lo
// que su transporte necesita y mantiene la firma del puerto estable (I4: core no
// cambia). La arista a mensajeria sí usa `bearer` explícito por no tener gemelo
// en proceso; aquí el context es el patrón correcto para una arista con ambos.
package tokenctx

import "context"

// claveBearer es la clave privada del contexto para el Bearer del usuario.
type claveBearer struct{}

// Con devuelve un contexto que lleva el Bearer dado (el valor crudo del header
// Authorization, incluido el prefijo "Bearer "). Un bearer vacío no se guarda.
func Con(ctx context.Context, bearer string) context.Context {
	if bearer == "" {
		return ctx
	}
	return context.WithValue(ctx, claveBearer{}, bearer)
}

// De extrae el Bearer del contexto. Devuelve "" si no hay ninguno; el adaptador
// HTTP trata la cadena vacía como "sin cabecera" (el destino responderá 401).
func De(ctx context.Context) string {
	b, _ := ctx.Value(claveBearer{}).(string)
	return b
}
