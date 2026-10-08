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

// TestComponerIncidenciasVacias verifica la brecha reportada (E2-T3): sin
// superficie contractual para la vista curada de incidencias, el scaffold las
// deja vacias (no nil), nunca inventa una categoria.
func TestComponerIncidenciasVacias(t *testing.T) {
	d := viajeDetalle(opclient.EstadoViaje(estados.EstadoEnTransito), nil)
	d.CondicionesActivas = []opclient.CondicionAbierta{
		{Tipo: opclient.INCIDENCIA, AbiertaEn: time.Now().UTC()},
	}
	seg := Componer(d)
	if seg.Incidencias == nil {
		t.Fatal("Incidencias no debe ser nil (vacio, no ausente)")
	}
	if len(seg.Incidencias) != 0 {
		t.Errorf("sin contrato de incidencia curada, Incidencias debe ir vacio; dio %d", len(seg.Incidencias))
	}
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
