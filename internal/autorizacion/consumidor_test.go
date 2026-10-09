//go:build nats

// Prueba del consumidor de revocación contra NATS real (principle-prove-it-works).
// Build tag "nats" para no romper CI sin broker.
//
//	docker run --rm -p 4222:4222 -p 8222:8222 nats:2-alpine -js -m 8222
//	go test -tags nats ./internal/autorizacion/
package autorizacion

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AI27-Tech/pf-pkg-platform-go/bus"
	busnats "github.com/AI27-Tech/pf-pkg-platform-go/bus/nats"
)

func urlNATS() string {
	if u := os.Getenv("NATS_URL"); u != "" {
		return u
	}
	return "nats://127.0.0.1:4222"
}

// TestConsumidorMarcaProyeccion verifica el tramo consumidor: una difusión de
// sesion.revocada por el broadcast marca la sesión en la proyección, de modo que
// el middleware (Revocada) la rechace después (ADR 26 §3).
func TestConsumidorMarcaProyeccion(t *testing.T) {
	c, err := busnats.Conectar(urlNATS())
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	defer c.Cerrar()

	proy := NuevaProyeccion()
	senal := NuevaSenalCanal(time.Now)

	// Antes de la difusión, la sesión no consta revocada.
	if proy.Revocada(context.Background(), "sesion-x") {
		t.Fatal("la proyección no debería conocer la sesión todavía")
	}

	w, err := NuevoConsumidorRevocacion(c, proy, senal)
	if err != nil {
		t.Fatalf("nuevo consumidor: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	time.Sleep(300 * time.Millisecond) // deja que se suscriba

	// Difunde la revocación, como lo haría el reemisor.
	err = c.Broadcast(ctx, bus.Sobre{
		EventoID:     "rev-1",
		Tipo:         "sesion.revocada",
		Version:      1,
		AgregadoID:   "sesion-x", // clave de orden = sesion_id
		Sensibilidad: bus.SensibilidadAmplia,
		OcurridoEn:   time.Now().UTC(),
		Payload:      []byte(`{"sesion_id":"sesion-x"}`),
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}

	// La proyección debe acabar marcando la sesión.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if proy.Revocada(context.Background(), "sesion-x") {
			return // éxito
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("la proyección nunca marcó la sesión: el consumidor no procesó la difusión")
}

// TestHeartbeatRefrescaSinMarcarTombstone verifica la discriminación por
// inclusión: un sobre de heartbeat refresca la señal del canal pero NO marca un
// tombstone (no es una revocación). Es lo que permite medir frescura sin tratar
// el latido como una sesión revocada.
func TestHeartbeatRefrescaSinMarcarTombstone(t *testing.T) {
	c, err := busnats.Conectar(urlNATS())
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	defer c.Cerrar()

	proy := NuevaProyeccion()
	senal := NuevaSenalCanal(time.Now)
	antes := senal.UltimaSenal()

	w, err := NuevoConsumidorRevocacion(c, proy, senal)
	if err != nil {
		t.Fatalf("nuevo consumidor: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	time.Sleep(300 * time.Millisecond)

	time.Sleep(10 * time.Millisecond) // garantiza que el reloj avance
	// Un tipo que NO es sesion.revocada: representa el heartbeat del canal. Se usa
	// el literal (no busnats.TipoHeartbeat) a propósito: la propiedad bajo prueba
	// es "cualquier sobre que no sea sesion.revocada refresca sin marcar
	// tombstone", y no debe acoplar bff a la versión de platform-go que define esa
	// constante. El consumidor discrimina por inclusión (sesion.revocada), no por
	// el valor del heartbeat.
	err = c.Broadcast(ctx, bus.Sobre{
		Tipo:         "bus.heartbeat",
		OcurridoEn:   time.Now().UTC(),
		Sensibilidad: bus.SensibilidadRestringida,
	})
	if err != nil {
		t.Fatalf("broadcast heartbeat: %v", err)
	}

	// La señal debe avanzar; ningún tombstone debe aparecer.
	deadline := time.Now().Add(3 * time.Second)
	refrescó := false
	for time.Now().Before(deadline) {
		if senal.UltimaSenal().After(antes) {
			refrescó = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !refrescó {
		t.Fatal("el heartbeat debió refrescar la señal del canal")
	}
	// El heartbeat no tiene AgregadoID; aunque lo tuviera, no es sesion.revocada,
	// así que no debe constar como revocado.
	if proy.Revocada(context.Background(), "") {
		t.Error("un heartbeat no debe marcar un tombstone")
	}
}
