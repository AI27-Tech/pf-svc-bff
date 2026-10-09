package bff

import (
	"net/http"
	"testing"

	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
)

// TestNewSinBusNoCreaConsumidor: en desarrollo sin bus, New no construye el
// consumidor y Workers queda vacío. bff sirve con verificación de firma, sin
// tombstones.
func TestNewSinBusNoCreaConsumidor(t *testing.T) {
	m, err := New(Pools{}, Deps{})
	if err != nil {
		t.Fatalf("New sin bus devolvió error: %v", err)
	}
	if len(m.Workers()) != 0 {
		t.Errorf("sin bus no debería haber workers, hubo %d", len(m.Workers()))
	}
}

// TestRoutesConRevocacionNilNoPanic: cuando Revocacion es nil, Routes monta las
// rutas protegidas sin hacer panic. Protege contra el gotcha del puntero nil
// envuelto en interfaz: el middleware debe recibir un RevocationChecker nil de
// interfaz y omitir la consulta de tombstones, no desreferenciar un *Proyeccion
// nil en cada petición.
func TestRoutesConRevocacionNilNoPanic(t *testing.T) {
	m, err := New(Pools{}, Deps{}) // Revocacion nil, Broadcaster nil
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	// No debe hacer panic al montar con proyección nil.
	m.Routes(mux)
}

// TestWorkersConProyeccion: si la raíz inyecta proyección pero no Broadcaster,
// tampoco se crea consumidor (hace falta el bus para suscribir). Documenta que
// ambas dependencias son necesarias.
func TestWorkersSinBroadcasterNoCreaConsumidor(t *testing.T) {
	m, err := New(Pools{}, Deps{Revocacion: autorizacion.NuevaProyeccion()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(m.Workers()) != 0 {
		t.Errorf("sin Broadcaster no debería haber consumidor, hubo %d workers", len(m.Workers()))
	}
}
