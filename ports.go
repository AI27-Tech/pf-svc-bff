package bff

import (
	"time"

	"github.com/AI27-Tech/pf-svc-bff/internal/agregacion"
)

// Los puertos de salida son interfaces minimas que bff declara como consumidor
// (§A.5 R2; regla 2 de 00). Se definen en internal/agregacion, donde se usan, y
// se re-exportan aqui como alias para que la superficie publica del paquete bff
// los exponga sin obligar a la raiz a importar internal/. Tipadas SOLO con tipos
// de pf-pkg-contracts, nunca con tipos de internal/ (regla 6 de 00).

// IdentidadPort es la arista a identidad. Emite, refresca y revoca sesion, y
// provee el snapshot de revocadas y la consulta de sesion para el fallback de
// ADR 26. NUNCA verifica tokens: la verificacion es local, con auth.Verifier.
type IdentidadPort = agregacion.IdentidadPort

// OperacionPort es la arista a operacion: crear viaje, listar por alcance y
// obtener detalle. Sin transiciones internas ni asignacion (bff.md (f)).
type OperacionPort = agregacion.OperacionPort

// Presupuestos de latencia por dependencia (bff.md (i)): el agregador no degrada
// una vista ajena, cada arista tiene su propio presupuesto y pool. Los VALORES
// concretos no estan en las fuentes y NO se inventan: quedan como configuracion
// con este comentario que lo dice. Los que siguen son marcadores de posicion
// para que la estructura (un presupuesto por puerto) exista desde ya; la raiz
// los sobrescribe por configuracion.
const (
	// PresupuestoOperacion es el presupuesto corto hacia operacion: sin reintento
	// ciego, fallo visible y marcado (bff.md (i)). Valor pendiente de configuracion.
	PresupuestoOperacion = 2 * time.Second
	// PresupuestoIdentidad es el presupuesto corto en emision y refresco de sesion;
	// fuera de la ruta de verificacion (bff.md (i)). Valor pendiente de configuracion.
	PresupuestoIdentidad = 2 * time.Second
)
