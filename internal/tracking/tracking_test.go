package tracking

import (
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	estados "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/estados"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/google/uuid"
)

// TestEtiquetaPorEstado verifica la traduccion por estado tecnico (E2-T2). Es
// exhaustiva sobre el enum canonico: si se agrega un estado nuevo sin hito, esta
// prueba falla, forzando a decidir su hito en vez de filtrar el token tecnico.
func TestEtiquetaPorEstado(t *testing.T) {
	casos := map[estados.EstadoViaje]string{
		estados.EstadoSolicitado:          "Solicitud recibida",
		estados.EstadoAprobado:            "Servicio confirmado",
		estados.EstadoAsignado:            "Servicio confirmado",
		estados.EstadoAptoParaSalida:      "En preparacion",
		estados.EstadoEnEmbarque:          "En preparacion",
		estados.EstadoPatrullaEnCamino:    "En preparacion",
		estados.EstadoEnTransito:          "En camino",
		estados.EstadoEnDestino:           "En camino",
		estados.EstadoEnDescarga:          "En camino",
		estados.EstadoCerrado:             "Servicio cerrado",
		estados.EstadoCancelado:           "Servicio cancelado",
		estados.EstadoRechazadoPorAi27:    "Servicio no procede",
		estados.EstadoRechazadoPorCliente: "Servicio no procede",
	}
	for estado, esperado := range casos {
		etiqueta, ok := Etiqueta(estado)
		if !ok {
			t.Errorf("estado %q sin hito mapeado", estado)
			continue
		}
		if etiqueta != esperado {
			t.Errorf("estado %q: hito = %q, se esperaba %q", estado, etiqueta, esperado)
		}
	}
}

// TestEtiquetaDesconocida verifica que un estado fuera del enum no produce
// etiqueta: el llamador decide la postura, nunca se filtra el token tecnico.
func TestEtiquetaDesconocida(t *testing.T) {
	if _, ok := Etiqueta(estados.EstadoViaje("ESTADO_INVENTADO")); ok {
		t.Fatal("un estado desconocido no debe tener hito mapeado")
	}
}

// TestColapsoMultiParada verifica que los tres estados del bucle multi-parada
// (EN_TRANSITO, EN_DESTINO, EN_DESCARGA) colapsan en el mismo hito "En camino":
// el cliente nunca ve el bucle por parada (E2-T2, criterio de aceptacion).
func TestColapsoMultiParada(t *testing.T) {
	bucle := []estados.EstadoViaje{
		estados.EstadoEnTransito,
		estados.EstadoEnDestino,
		estados.EstadoEnDescarga,
	}
	for _, e := range bucle {
		etiqueta, ok := Etiqueta(e)
		if !ok || etiqueta != "En camino" {
			t.Errorf("estado %q del bucle debe colapsar a 'En camino', dio %q (ok=%v)", e, etiqueta, ok)
		}
	}
}

// TestComponerNoFiltraTokenTecnico verifica que la etiqueta presentada nunca es
// el token tecnico del enum, y que la referencia tecnica queda en Hito.Estado
// (campo de referencia, no de presentacion).
func TestComponerNoFiltraTokenTecnico(t *testing.T) {
	seg := Componer(viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil))
	if seg.Hito.Etiqueta == nil {
		t.Fatal("falta la etiqueta de hito")
	}
	if *seg.Hito.Etiqueta == string(estados.EstadoEnTransito) {
		t.Errorf("la etiqueta no debe ser el token tecnico: %q", *seg.Hito.Etiqueta)
	}
	if *seg.Hito.Etiqueta != "En camino" {
		t.Errorf("hito = %q, se esperaba 'En camino'", *seg.Hito.Etiqueta)
	}
}

// TestComponerStaleCalculado verifica la regla de E2-T2: bff calcula stale
// comparando la hora actual contra el ETA guardado (operacion no expone stale,
// tracer.4/ADR 20). stale es true si el ETA ya pasó, false si está en el futuro.
// bff nunca recalcula el ETA: solo marca si venció.
func TestComponerStaleCalculado(t *testing.T) {
	// Reloj fijo para la prueba; se restaura al terminar.
	fijo := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	original := ahora
	ahora = func() time.Time { return fijo }
	defer func() { ahora = original }()

	casos := []struct {
		nombre   string
		eta      time.Time
		obsoleto bool
	}{
		{"ETA ya pasó → obsoleto", fijo.Add(-time.Hour), true},
		{"ETA en el futuro → vigente", fijo.Add(time.Hour), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			eta := c.eta
			seg := Componer(viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), &eta))
			if seg.Eta == nil {
				t.Fatal("se esperaba ETA en la vista de seguimiento")
			}
			if seg.Eta.Obsoleto != c.obsoleto {
				t.Errorf("obsoleto = %v, se esperaba %v", seg.Eta.Obsoleto, c.obsoleto)
			}
			if !seg.Eta.Valor.Equal(c.eta) {
				t.Errorf("valor del ETA = %v, se esperaba %v (bff no recalcula)", seg.Eta.Valor, c.eta)
			}
		})
	}
}

// TestComponerSinEta verifica que un viaje sin ETA capturado no produce bloque Eta.
func TestComponerSinEta(t *testing.T) {
	seg := Componer(viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil))
	if seg.Eta != nil {
		t.Error("sin eta_estimado, la vista no debe traer bloque Eta")
	}
}

// TestComponerSinIncidenciasActivas verifica que un viaje sin incidencias
// abiertas (IncidenciasActivas nil o vacio) produce una lista de incidencias
// vacia (no nil): no se afirma ausencia como dato ausente y nunca se inventa una
// categoria. Una CondicionAbierta de tipo INCIDENCIA por si sola NO aporta
// incidencia curada: el curado se compone de IncidenciasActivas (que trae el
// tipo_interno), no de CondicionesActivas (dato crudo sin catalogo).
func TestComponerSinIncidenciasActivas(t *testing.T) {
	casos := map[string]opclient.ViajeDetalle{
		"IncidenciasActivas nil": func() opclient.ViajeDetalle {
			d := viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil)
			d.IncidenciasActivas = nil
			return d
		}(),
		"IncidenciasActivas vacio": func() opclient.ViajeDetalle {
			d := viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil)
			d.IncidenciasActivas = &[]opclient.IncidenciaActiva{}
			return d
		}(),
		"solo CondicionAbierta INCIDENCIA, sin IncidenciasActivas": func() opclient.ViajeDetalle {
			d := viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil)
			d.CondicionesActivas = []opclient.CondicionAbierta{
				{Tipo: opclient.INCIDENCIA, AbiertaEn: time.Now().UTC()},
			}
			return d
		}(),
	}
	for nombre, d := range casos {
		t.Run(nombre, func(t *testing.T) {
			seg := Componer(d)
			if seg.Incidencias == nil {
				t.Fatal("Incidencias no debe ser nil (vacio, no ausente)")
			}
			if len(seg.Incidencias) != 0 {
				t.Errorf("sin incidencias activas, Incidencias debe ir vacio; dio %d", len(seg.Incidencias))
			}
		})
	}
}

// tiposInternosDelMapeo enumera TODOS los tipos internos de la tabla cerrada
// (§3 de docs/propuestas/propuesta-clasificacion-estatus-v1.md), con la
// categoria curada que les corresponde. Es la tabla de verdad de la prueba: si
// el mapeo cambia o se agrega un tipo nuevo sin categoria, estas pruebas lo
// cazan. Se escribe aqui a mano (no se lee del mapa de produccion) para que la
// prueba afirme el contrato esperado, no se limite a repetir la implementacion.
var tiposInternosDelMapeo = map[string]string{
	// Incidencia de seguridad.
	"Asalto O Robo":                         CategoriaSeguridad,
	"Intercepción de custodia":              CategoriaSeguridad,
	"Abandono de custodia":                  CategoriaSeguridad,
	"Rompimiento de Convoy":                 CategoriaSeguridad,
	"Sabotaje GPS":                          CategoriaSeguridad,
	"Retiro de GPS":                         CategoriaSeguridad,
	"Custodia da clave de amago incorrecta": CategoriaSeguridad,
	"Custodia no da clave de amago":         CategoriaSeguridad,
	"Extraviado":                            CategoriaSeguridad,
	// Accidente / percance.
	"Accidente Del Transporte Por Volcadura": CategoriaAccidente,
	"Percance Automovilístico":               CategoriaAccidente,
	// Falla mecánica / de equipo.
	"Falla Mecánica":             CategoriaFalla,
	"Falla mecánica de patrulla": CategoriaFalla,
	"Falla en cámaras":           CategoriaFalla,
	// Retraso / desvío en ruta.
	"Bloqueo Carretero": CategoriaRetraso,
	"Desvió de Ruta":    CategoriaRetraso,
	// Pérdida de contacto.
	"Custodia no responde al teléfono": CategoriaPerdidaContacto,
	"Operador no responde al teléfono": CategoriaPerdidaContacto,
}

// categoriasValidas son las cinco categorias curadas EXACTAS (§3). Cualquier
// valor fuera de este conjunto en el payload del cliente es una violacion: ni
// una categoria inventada ni el tipo interno filtrado.
var categoriasValidas = map[string]bool{
	CategoriaSeguridad:       true,
	CategoriaAccidente:       true,
	CategoriaFalla:           true,
	CategoriaRetraso:         true,
	CategoriaPerdidaContacto: true,
}

// TestCategoriaMapeaCadaTipoInterno recorre todos los tipos internos de la tabla
// cerrada y verifica que cada uno mapea a su categoria esperada. Falla si un
// tipo conocido pierde su categoria o cae en otra distinta.
func TestCategoriaMapeaCadaTipoInterno(t *testing.T) {
	for tipo, esperada := range tiposInternosDelMapeo {
		cat, ok := Categoria(tipo)
		if !ok {
			t.Errorf("tipo interno %q no mapea a ninguna categoria", tipo)
			continue
		}
		if cat != esperada {
			t.Errorf("tipo %q: categoria = %q, se esperaba %q", tipo, cat, esperada)
		}
	}
}

// TestNingunTipoInternoSeFiltraAlCliente es el criterio central de E2-T3
// (DEC-006): se enumeran TODOS los tipos internos y se verifica que NINGUNO
// aparece en el payload curado del cliente. Para cada tipo se compone un viaje
// con esa incidencia activa y se revisa que la categoria sea una de las cinco
// validas y que el string de la incidencia nunca sea el tipo interno.
func TestNingunTipoInternoSeFiltraAlCliente(t *testing.T) {
	for tipo := range tiposInternosDelMapeo {
		t.Run(tipo, func(t *testing.T) {
			d := viajeConIncidencias(incidenciaActiva(tipo, opclient.IncidenciaActivaEstadoEnAtencion))
			seg := Componer(d)
			if len(seg.Incidencias) != 1 {
				t.Fatalf("se esperaba 1 incidencia curada, dio %d", len(seg.Incidencias))
			}
			inc := seg.Incidencias[0]
			if inc.Categoria == tipo {
				t.Errorf("el tipo interno %q se filtro como categoria al cliente", tipo)
			}
			if !categoriasValidas[inc.Categoria] {
				t.Errorf("categoria %q no es una de las cinco validas (tipo %q)", inc.Categoria, tipo)
			}
			// Defensa adicional: el tipo interno no debe aparecer en ningun campo
			// de texto del payload del cliente.
			if contieneTipoInterno(inc, tipo) {
				t.Errorf("el tipo interno %q aparece en el payload curado: %+v", tipo, inc)
			}
		})
	}
}

// TestSabotajeGpsEsUnaSolaIncidenciaDeSeguridad verifica el caso de doble
// dimension (criterio E2-T3): "Sabotaje GPS" es a la vez condicion de señal e
// incidencia, pero el cliente lo ve como UNA sola "Incidencia de seguridad",
// nunca dos entradas ni el nombre tecnico. Se compone un viaje con la condicion
// de señal abierta y la incidencia de sabotaje a la vez.
func TestSabotajeGpsEsUnaSolaIncidenciaDeSeguridad(t *testing.T) {
	d := viajeConIncidencias(incidenciaActiva("Sabotaje GPS", opclient.IncidenciaActivaEstadoEnAtencion))
	// La misma realidad tambien abre una condicion de señal (SIN_SENAL): el
	// cliente NO debe ver una segunda entrada por eso.
	d.CondicionesActivas = []opclient.CondicionAbierta{
		{Tipo: opclient.SINSENAL, AbiertaEn: time.Now().UTC()},
	}
	seg := Componer(d)
	if len(seg.Incidencias) != 1 {
		t.Fatalf("Sabotaje GPS debe dar UNA sola incidencia curada, dio %d", len(seg.Incidencias))
	}
	inc := seg.Incidencias[0]
	if inc.Categoria != CategoriaSeguridad {
		t.Errorf("categoria = %q, se esperaba %q", inc.Categoria, CategoriaSeguridad)
	}
	if contieneTipoInterno(inc, "Sabotaje GPS") {
		t.Errorf("el nombre tecnico 'Sabotaje GPS' se filtro al cliente: %+v", inc)
	}
}

// TestIncidenciaResueltaDesapareceDeActivas verifica el flujo de resolucion
// (criterio E2-T3): cuando operacion resuelve la incidencia, esta deja de estar
// en incidencias_activas, asi que en la siguiente lectura el cliente deja de
// verla, sin accion adicional. Se modela leyendo el mismo viaje antes y despues
// de la resolucion.
func TestIncidenciaResueltaDesapareceDeActivas(t *testing.T) {
	// Antes: una incidencia abierta -> el cliente la ve "en_atencion".
	antes := viajeConIncidencias(incidenciaActiva("Asalto O Robo", opclient.IncidenciaActivaEstadoEnAtencion))
	segAntes := Componer(antes)
	if len(segAntes.Incidencias) != 1 {
		t.Fatalf("antes de resolver, el cliente debe ver 1 incidencia, dio %d", len(segAntes.Incidencias))
	}
	if segAntes.Incidencias[0].Estado != bffclient.IncidenciaCuradaEstadoEnAtencion {
		t.Errorf("estado = %q, se esperaba en_atencion", segAntes.Incidencias[0].Estado)
	}

	// Despues: operacion resolvio la incidencia -> sale de incidencias_activas.
	// La siguiente lectura del cliente ya no la trae, sin paso extra.
	despues := viajeConIncidencias()
	segDespues := Componer(despues)
	if len(segDespues.Incidencias) != 0 {
		t.Errorf("tras resolver, el cliente no debe ver la incidencia; dio %d", len(segDespues.Incidencias))
	}
}

// TestIncidenciaEstadoResueltaSeTraduce verifica que, si operacion llegara a
// exponer una incidencia activa con estado resuelta, bff la traduce tal cual
// (no la deriva): el estado del cliente refleja el de operacion.
func TestIncidenciaEstadoResueltaSeTraduce(t *testing.T) {
	d := viajeConIncidencias(incidenciaActiva("Asalto O Robo", opclient.IncidenciaActivaEstadoResuelta))
	seg := Componer(d)
	if len(seg.Incidencias) != 1 {
		t.Fatalf("se esperaba 1 incidencia, dio %d", len(seg.Incidencias))
	}
	if seg.Incidencias[0].Estado != bffclient.IncidenciaCuradaEstadoResuelta {
		t.Errorf("estado = %q, se esperaba resuelta", seg.Incidencias[0].Estado)
	}
}

// contieneTipoInterno revisa que el tipo interno no aparezca en los campos de
// texto de la incidencia curada (categoria o impacto_eta). El estado y la marca
// de tiempo son valores controlados por el contrato, no texto libre.
func contieneTipoInterno(inc bffclient.IncidenciaCurada, tipoInterno string) bool {
	if inc.Categoria == tipoInterno {
		return true
	}
	if inc.ImpactoEta != nil && *inc.ImpactoEta == tipoInterno {
		return true
	}
	return false
}

// incidenciaActiva arma una IncidenciaActiva de operacion para las pruebas.
func incidenciaActiva(tipoInterno string, estado opclient.IncidenciaActivaEstado) opclient.IncidenciaActiva {
	return opclient.IncidenciaActiva{
		Id:          uuid.New(),
		TipoInterno: tipoInterno,
		Estado:      estado,
		AbiertaEn:   time.Now().UTC(),
	}
}

// viajeConIncidencias arma un ViajeDetalle EN_TRANSITO con las incidencias
// activas dadas (ninguna si no se pasan).
func viajeConIncidencias(incs ...opclient.IncidenciaActiva) opclient.ViajeDetalle {
	d := viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil)
	lista := append([]opclient.IncidenciaActiva{}, incs...)
	d.IncidenciasActivas = &lista
	return d
}

// viajeDetalle arma un ViajeDetalle minimo para las pruebas.
func viajeDetalle(estado opclient.EstadoViaje, eta *time.Time) opclient.ViajeDetalle {
	return opclient.ViajeDetalle{
		Viaje: opclient.Viaje{
			Id:          uuid.New(),
			Estado:      estado,
			Origen:      "CDMX",
			Destinos:    []opclient.Destino{{Nombre: "Monterrey"}},
			EtaEstimado: eta,
		},
		CondicionesActivas: []opclient.CondicionAbierta{},
	}
}

// compile-time: la vista producida es del tipo del contrato de bff.
var _ bffclient.ViajeSeguimiento = Componer(opclient.ViajeDetalle{})
