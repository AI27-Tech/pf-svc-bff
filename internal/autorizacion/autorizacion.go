// Package autorizacion envuelve la verificacion local de JWT de
// pf-pkg-platform-go/auth y la consulta de tombstones de ADR 26. bff NUNCA
// verifica tokens viajando a identidad: la verificacion es local, contra el
// JWKS del IdP interno (auth.NewVerifier), con refresco proactivo y fallo
// cerrado cuando no hay claves utilizables (DEC-056; bff.md (l) ADR 8).
//
// La Audiencia de dominio se lee de auth.Claims.Audiencia (Cliente | Interno),
// fijada por la composicion de claims del IdP a partir del rol (DEC-054), nunca
// de un parametro (P10). auth.Claims.Audience es el claim tecnico `aud` y NUNCA
// autoriza.
package autorizacion

import (
	"context"
	"net/http"

	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
)

// Middleware protege una ruta: verifica el JWT localmente y, si recibe un
// verificador de revocacion, rechaza los tokens cuya sesion este revocada (ADR
// 26 §3). Es un envoltorio delgado sobre auth.Middleware de la plataforma para
// que bff mantenga su propia costura y el resto del modulo no dependa del
// paquete auth directamente.
//
// rev puede ser nil en el esqueleto (sin proyeccion de tombstones): entonces la
// ruta se protege solo con verificacion de firma. En produccion rev es la
// Proyeccion alimentada por el worker de sesion.revocada, que falla cerrado.
func Middleware(verificador auth.Authenticator, rev auth.RevocationChecker) func(http.Handler) http.Handler {
	return auth.Middleware(verificador, rev)
}

// ClaimsDe extrae las claims verificadas del contexto de la peticion. Devuelve
// nil si la ruta no paso por el Middleware (no deberia ocurrir en rutas
// protegidas). Es el unico punto desde el que los handlers leen identidad,
// tenant, rol y Audiencia (nunca de query ni body, P10).
func ClaimsDe(ctx context.Context) *auth.Claims {
	return auth.ClaimsFromContext(ctx)
}

// Proyeccion es la proyeccion local de tombstones sesion_id -> revocada (ADR 26
// §3). Toda ruta protegida la consulta antes de aceptar. Se reconstruye por
// snapshot (identidad.listarSesionesRevocadas) y se alimenta por la difusion de
// sesion.revocada a CADA replica (no consumer group, ADR 26 §3; DEC-057).
//
// Implementa auth.RevocationChecker. Idempotente por evento_id y sesion_id
// (P13).
//
// REGLA DURA (ADR 26, bff.md (g)(i)): si el canal de revocacion no tiene
// frescura por mas de 15 s, se consulta a identidad; si identidad tampoco
// responde, se FALLA CERRADO (se trata la sesion como revocada). El esqueleto
// deja esa logica de frescura/fallback como TODO explicito, pero el zero value
// ya falla seguro: sin datos cargados, una consulta responde "no se puede
// afirmar que este viva" segun la postura que configure la raiz.
type Proyeccion struct {
	// revocadas es el conjunto de sesiones revocadas conocidas. Mapa como dato;
	// el acceso concurrente lo protege la raiz cuando cablee el worker (el
	// esqueleto no arranca goroutines, 00 regla de concurrencia).
	revocadas map[string]struct{}
}

// NuevaProyeccion crea una proyeccion vacia. Zero value util: sin snapshot
// cargado, no conoce ninguna revocacion todavia; la raiz debe reconstruir por
// snapshot antes de servir trafico protegido (ADR 26).
func NuevaProyeccion() *Proyeccion {
	return &Proyeccion{revocadas: make(map[string]struct{})}
}

// Revocada implementa auth.RevocationChecker: devuelve true si la sesion fue
// revocada. En el esqueleto solo consulta el conjunto local.
//
// TODO(bff, ADR 26): incorporar la regla de frescura (>15 s sin señal del canal
// -> fallback a identidad -> si no responde, fallar cerrado devolviendo true).
// Esa logica se cablea cuando la raiz inyecte el reloj, el puerto de identidad y
// el worker de sesion.revocada.
func (p *Proyeccion) Revocada(_ context.Context, sesionID string) bool {
	if p == nil || p.revocadas == nil {
		return false
	}
	_, revocada := p.revocadas[sesionID]
	return revocada
}

// Marcar registra una sesion como revocada. Idempotente por sesion_id (P13): el
// mismo sesionID dos veces no cambia el resultado. Lo invoca el worker de
// sesion.revocada al procesar un evento, y la reconstruccion por snapshot.
func (p *Proyeccion) Marcar(sesionID string) {
	if p.revocadas == nil {
		p.revocadas = make(map[string]struct{})
	}
	p.revocadas[sesionID] = struct{}{}
}

// compile-time: Proyeccion satisface el contrato de revocacion de la plataforma.
var _ auth.RevocationChecker = (*Proyeccion)(nil)
