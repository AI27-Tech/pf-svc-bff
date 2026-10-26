package descarga

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	estados "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/estados"
	"github.com/google/uuid"
)

// Componer es una funcion pura: estas pruebas afirman el expediente compuesto
// contra valores literales, mas la invariante de que las listas nunca son nil y
// que el expediente no serializa datos excluidos.

func TestComponerMapeaFichaBitacoraYAuditoria(t *testing.T) {
	id := uuid.MustParse("12121212-1212-1212-1212-121212121212")
	cid := uuid.MustParse("c2222222-2222-2222-2222-222222222222")
	fhs := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	ts := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)

	detalle := opclient.ViajeDetalle{
		Viaje: opclient.Viaje{
			Id:                id,
			Estado:            opclient.EstadoViaje(estados.EstadoEnTransito),
			ClienteId:         &cid,
			Origen:            "CDMX",
			Destinos:          []opclient.Destino{{Nombre: "Monterrey", NumeroEnvio: strPtr("ENV-1")}},
			FechaHoraServicio: &fhs,
		},
	}
	bitacora := []opclient.EntradaBitacora{
		{Id: uuid.New(), ViajeId: id, Autor: "mon-1", Texto: "salida", Ts: ts},
	}
	auditoria := []opclient.RegistroAuditoria{
		{Actor: "jefe-1", Entidad: "asignacion", EntidadId: uuid.New(), Ts: ts},
	}

	generado := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	got := Componer(detalle, bitacora, auditoria, generado)

	if got.ViajeId != id {
		t.Errorf("viaje_id = %v, se esperaba %v", got.ViajeId, id)
	}
	if got.Ficha.Origen != "CDMX" || got.Ficha.Estado != string(estados.EstadoEnTransito) {
		t.Errorf("ficha mal compuesta: %+v", got.Ficha)
	}
	if len(got.Ficha.Destinos) != 1 || got.Ficha.Destinos[0].Nombre != "Monterrey" {
		t.Errorf("destinos de ficha mal mapeados: %+v", got.Ficha.Destinos)
	}
	if got.Ficha.ClienteId == nil || *got.Ficha.ClienteId != cid {
		t.Errorf("cliente_id = %v, se esperaba %v", got.Ficha.ClienteId, cid)
	}
	if len(got.Bitacora) != 1 || got.Bitacora[0].Autor != "mon-1" || got.Bitacora[0].Texto != "salida" {
		t.Errorf("bitacora mal compuesta: %+v", got.Bitacora)
	}
	if len(got.Auditoria) != 1 || got.Auditoria[0].Actor != "jefe-1" || got.Auditoria[0].Entidad != "asignacion" {
		t.Errorf("auditoria mal compuesta: %+v", got.Auditoria)
	}
	if !got.GeneradoEn.Equal(generado) {
		t.Errorf("generado_en = %v, se esperaba %v", got.GeneradoEn, generado)
	}
}

// TestComponerListasVaciasNuncaNil: sin bitacora ni auditoria, las listas del
// expediente son vacias (no nil): una lista vacia no es ausencia de dato.
func TestComponerListasVaciasNuncaNil(t *testing.T) {
	id := uuid.New()
	got := Componer(opclient.ViajeDetalle{Viaje: opclient.Viaje{Id: id, Origen: "A"}}, nil, nil, time.Now())
	if got.Bitacora == nil {
		t.Error("bitacora no debe ser nil")
	}
	if got.Auditoria == nil {
		t.Error("auditoria no debe ser nil")
	}
	if len(got.Bitacora) != 0 || len(got.Auditoria) != 0 {
		t.Errorf("listas deben estar vacias: bitacora=%d auditoria=%d", len(got.Bitacora), len(got.Auditoria))
	}
}

// TestComponerNoSerializaDatosExcluidos: el JSON del expediente nunca contiene
// cuenta espejo ni clave_amago (DEC-009/DEC-039). No existen en el contrato de
// DescargaViaje, asi que el marshaling no puede emitirlos.
func TestComponerNoSerializaDatosExcluidos(t *testing.T) {
	id := uuid.New()
	ts := time.Now().UTC()
	got := Componer(
		opclient.ViajeDetalle{Viaje: opclient.Viaje{Id: id, Origen: "CDMX"}},
		[]opclient.EntradaBitacora{{Id: uuid.New(), ViajeId: id, Autor: "mon", Texto: "nota", Ts: ts}},
		[]opclient.RegistroAuditoria{{Actor: "op", Entidad: "viajes", EntidadId: uuid.New(), Ts: ts}},
		ts,
	)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cuerpo := strings.ToLower(string(raw))
	for _, excluido := range []string{"espejo", "clave_amago", "amago"} {
		if strings.Contains(cuerpo, excluido) {
			t.Errorf("el expediente serializo el dato excluido %q: %s", excluido, string(raw))
		}
	}
}

func strPtr(s string) *string { return &s }
