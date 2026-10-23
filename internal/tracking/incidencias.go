package tracking

import (
	"strings"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
)

// Las cinco categorias curadas que ve el cliente (DEC-006,
// docs/propuestas/propuesta-clasificacion-estatus-v1.md §3). Son EXACTAS: el
// cliente nunca ve el tipo interno, solo una de estas cinco. No se agregan
// categorias fuera de esta tabla cerrada.
const (
	CategoriaSeguridad       = "Incidencia de seguridad"
	CategoriaAccidente       = "Accidente/percance"
	CategoriaFalla           = "Falla mecánica/de equipo"
	CategoriaRetraso         = "Retraso/desvío en ruta"
	CategoriaPerdidaContacto = "Pérdida de contacto"
)

// categoriaPorTipoInterno mapea cada tipo interno de incidencia (el string
// canonico del catalogo de estatus operativos, .kiro/sources/
// estatus-operativos-sistema-actual.md) a su categoria curada del cliente, segun
// la tabla CERRADA de §3 de la propuesta de clasificacion (DEC-006, confirmada
// por Producto el 2026-09-08). No se infieren categorias: solo agrupa los tipos
// que la tabla ya asigna.
//
// Es un dato (mapa), no una cadena de if/switch, para que la prueba recorra el
// catalogo completo y falle si aparece un tipo sin categoria. Las claves se
// comparan normalizadas (ver Categoria) para no romperse por mayusculas o
// espacios accidentales del catalogo.
//
// "Sabotaje GPS" y "Retiro de GPS" aparecen aqui como UNA sola incidencia de
// seguridad: aunque en la clasificacion tambien son condicion de señal (§5
// punto 3), el cliente los ve como una unica "Incidencia de seguridad", nunca
// dos entradas ni el nombre tecnico (criterio de aceptacion E2-T3).
var categoriaPorTipoInterno = map[string]string{
	// Incidencia de seguridad.
	normalizar("Asalto O Robo"):                         CategoriaSeguridad,
	normalizar("Intercepción de custodia"):              CategoriaSeguridad,
	normalizar("Abandono de custodia"):                  CategoriaSeguridad,
	normalizar("Rompimiento de Convoy"):                 CategoriaSeguridad,
	normalizar("Sabotaje GPS"):                          CategoriaSeguridad,
	normalizar("Retiro de GPS"):                         CategoriaSeguridad,
	normalizar("Custodia da clave de amago incorrecta"): CategoriaSeguridad,
	normalizar("Custodia no da clave de amago"):         CategoriaSeguridad,
	normalizar("Extraviado"):                            CategoriaSeguridad,
	// Accidente / percance.
	normalizar("Accidente Del Transporte Por Volcadura"): CategoriaAccidente,
	normalizar("Percance Automovilístico"):               CategoriaAccidente,
	// Falla mecánica / de equipo.
	normalizar("Falla Mecánica"):             CategoriaFalla,
	normalizar("Falla mecánica de patrulla"): CategoriaFalla,
	normalizar("Falla en cámaras"):           CategoriaFalla,
	// Retraso / desvío en ruta.
	normalizar("Bloqueo Carretero"): CategoriaRetraso,
	normalizar("Desvió de Ruta"):    CategoriaRetraso,
	// Pérdida de contacto.
	normalizar("Custodia no responde al teléfono"): CategoriaPerdidaContacto,
	normalizar("Operador no responde al teléfono"): CategoriaPerdidaContacto,
}

// normalizar reduce un tipo interno a una forma estable de comparacion:
// sin espacios al borde y en minusculas. El catalogo de origen no garantiza
// capitalizacion uniforme, asi que la clave del mapa se normaliza para que
// "Sabotaje GPS" y "sabotaje gps" caigan en la misma categoria. Nunca expone el
// valor: es solo para buscar en el mapa.
func normalizar(tipoInterno string) string {
	return strings.ToLower(strings.TrimSpace(tipoInterno))
}

// Categoria traduce un tipo interno de incidencia a su categoria curada del
// cliente. Devuelve la categoria y true si el tipo esta en la tabla cerrada; ""
// y false si no esta mapeado, para que el llamador decida la postura ante un
// tipo desconocido. NUNCA devuelve el tipo interno: ese es justamente el termino
// que DEC-006 prohibe filtrar al cliente.
func Categoria(tipoInterno string) (string, bool) {
	cat, ok := categoriaPorTipoInterno[normalizar(tipoInterno)]
	return cat, ok
}

// incidenciasCuradas traduce las incidencias activas que operacion expone
// (consumo interno, con tipo_interno) a la vista curada del cliente, aplicando
// el mapeo de categoria. Es pura: no hace E/S y no muta su entrada.
//
// Reglas (E2-T3, DEC-006, DEC-021):
//   - Cada incidencia activa produce UNA entrada curada: categoria + estado +
//     marca de tiempo. El cliente nunca ve el tipo_interno.
//   - El estado y la marca de tiempo vienen tal cual de operacion (en_atencion
//     para una incidencia abierta; resuelta cuando operacion lo marque). bff no
//     los deriva.
//   - La incidencia aparece en cuanto el operador la marca, sin paso extra de
//     "publicar al cliente" (DEC-021): es la misma lectura de condicion_viaje.
//   - impacto_eta se rellena solo si operacion lo registra; hoy ViajeDetalle no
//     trae impacto por incidencia, asi que queda nil (no se inventa).
//
// Un tipo_interno fuera de la tabla cerrada (tipo nuevo o desconocido) NO se
// filtra: cae en "Incidencia de seguridad", la categoria mas conservadora. Es la
// postura fail-safe de DEC-006 —nunca exponer el termino tecnico y nunca ocultar
// una incidencia activa al cliente—, sin inventar una sexta categoria. La tabla
// §3 es exhaustiva sobre el catalogo conocido, asi que esto solo aplica a un tipo
// que entre sin pasar por la clasificacion.
func incidenciasCuradas(activas []opclient.IncidenciaActiva) []bffclient.IncidenciaCurada {
	curadas := make([]bffclient.IncidenciaCurada, 0, len(activas))
	for _, a := range activas {
		categoria, ok := Categoria(a.TipoInterno)
		if !ok {
			categoria = CategoriaSeguridad
		}
		curadas = append(curadas, bffclient.IncidenciaCurada{
			Categoria:   categoria,
			Estado:      bffclient.IncidenciaCuradaEstado(a.Estado),
			MarcaTiempo: a.AbiertaEn,
			// ImpactoEta queda nil: operacion no expone impacto por incidencia en
			// ViajeDetalle (tracer.16). Se rellenara cuando el contrato lo traiga.
		})
	}
	return curadas
}
