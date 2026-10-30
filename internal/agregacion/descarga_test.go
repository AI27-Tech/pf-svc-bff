package agregacion

import (
	"net/http"
	"strings"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	estados "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/estados"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

// Estas pruebas llaman al handler de descarga como lo llama su usuario real: por
// una peticion HTTP con claims en el contexto, y afirman el resultado observable
// (status y cuerpo JSON) mas el efecto sobre los puertos (cuantas veces se pidio
// el rastro de auditoria). Ninguna pasaria con una forma vacia.

// detalleDescarga arma un ViajeDetalle minimo valido para componer el expediente.
func detalleDescarga(id opclient.Id) *opclient.ViajeDetalle {
	cid := uuid.MustParse("c1111111-1111-1111-1111-111111111111")
	return &opclient.ViajeDetalle{
		Viaje: opclient.Viaje{
			Id:        id,
			Estado:    opclient.EstadoViaje(estados.EstadoEnTransito),
			ClienteId: &cid,
			Origen:    "CDMX",
			Destinos:  []opclient.Destino{{Nombre: "Monterrey", NumeroEnvio: ptr("ENV-9")}},
		},
		CondicionesActivas: []opclient.CondicionAbierta{},
	}
}

// --- Autorizacion por audiencia (ANTES de componer nada) ---

// TestDescargarViajeClienteEs403AntesDeComponer: un usuario con audiencia
// Cliente recibe 403 y el handler NO toca operacion (ni lectura ni registro):
// el rechazo por audiencia ocurre antes de componer dato alguno (E6-T3).
func TestDescargarViajeClienteEs403AntesDeComponer(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	fakeOp := &fakeOperacion{detalle: detalleDescarga(id)}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "u-cliente", Audiencia: auth.AudienciaCliente})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, se esperaba 403 para audiencia cliente; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var e bffclient.Error
	decodificarCuerpo(t, rec, &e)
	if e.Codigo != "prohibido" {
		t.Errorf("codigo = %q, se esperaba 'prohibido'", e.Codigo)
	}
	// No se compuso nada: operacion no fue consultada ni se dejo rastro.
	if fakeOp.obtenerRecibio != (opclient.Id{}) {
		t.Error("no se debe leer el viaje cuando la audiencia es cliente (rechazo antes de componer)")
	}
	if fakeOp.registrarLlamado != 0 {
		t.Errorf("RegistrarDescargaViaje se llamo %d veces; no debe llamarse ante rechazo por audiencia", fakeOp.registrarLlamado)
	}
}

// TestDescargarViajeSinClaimsEs401: ruta protegida sin claims en el contexto.
func TestDescargarViajeSinClaimsEs401(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	fakeOp := &fakeOperacion{detalle: detalleDescarga(id)}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(), nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba 401 sin claims", rec.Code)
	}
	if fakeOp.registrarLlamado != 0 {
		t.Error("no debe registrarse descarga sin claims")
	}
}

// TestDescargarViajeFueraDeAlcanceSePropagaComo403: el alcance fino lo resuelve
// operacion; su 403 (fuera de alcance) se propaga tal cual y NO se deja rastro
// (no hubo descarga exitosa). El usuario es interno (pasa la guarda de audiencia).
func TestDescargarViajeFueraDeAlcanceSePropagaComo403(t *testing.T) {
	id := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	// operacion responde 403 en la primera lectura (obtenerViaje) -> el adaptador
	// lo traduce a un Prohibido de bff. Aqui se simula con ese error directamente.
	fakeOp := &fakeOperacion{err: platformErrProhibido()}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "op-1", Audiencia: auth.AudienciaInterno})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, se esperaba 403 por alcance aguas abajo; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if fakeOp.registrarLlamado != 0 {
		t.Error("no debe dejarse rastro si la descarga no fue exitosa (fuera de alcance)")
	}
}

// TestDescargarViajeInexistenteSePropagaComo404: un viaje inexistente devuelve
// 404 y no se deja rastro.
func TestDescargarViajeInexistenteSePropagaComo404(t *testing.T) {
	id := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	fakeOp := &fakeOperacion{err: platformerrors.NewNoEncontrado("viaje no existe")}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "op-1", Audiencia: auth.AudienciaInterno})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, se esperaba 404", rec.Code)
	}
	if fakeOp.registrarLlamado != 0 {
		t.Error("no debe dejarse rastro de la descarga de un expediente inexistente")
	}
}

// --- Descarga exitosa ---

// TestDescargarViajeExitosaComponeExpedienteYDejaRastroUnaVez verifica el camino
// feliz: un usuario interno obtiene el expediente compuesto (ficha + bitacora +
// auditoria + generado_en) y operacion deja el rastro EXACTAMENTE una vez.
func TestDescargarViajeExitosaComponeExpedienteYDejaRastroUnaVez(t *testing.T) {
	id := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	fakeOp := &fakeOperacion{
		detalle: detalleDescarga(id),
		bitacoraPaginas: []*opclient.PaginaBitacora{{
			Items: []opclient.EntradaBitacora{
				{Id: uuid.New(), ViajeId: id, Autor: "monitorista-1", Texto: "salida registrada", Ts: time.Now().UTC()},
			},
		}},
		auditoriaPaginas: []*opclient.PaginaAuditoria{{
			Items: []opclient.RegistroAuditoria{
				{Actor: "jefe-1", Entidad: "asignacion", EntidadId: uuid.New(), Ts: time.Now().UTC()},
			},
		}},
		registrarViaje: &opclient.Viaje{Id: id},
	}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "op-1", Audiencia: auth.AudienciaInterno})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.DescargaViaje
	decodificarCuerpo(t, rec, &got)
	if got.ViajeId != id {
		t.Errorf("viaje_id = %v, se esperaba %v", got.ViajeId, id)
	}
	if got.Ficha.Origen != "CDMX" {
		t.Errorf("ficha.origen = %q, se esperaba CDMX", got.Ficha.Origen)
	}
	if len(got.Bitacora) != 1 || got.Bitacora[0].Autor != "monitorista-1" {
		t.Errorf("bitacora mal compuesta: %+v", got.Bitacora)
	}
	if len(got.Auditoria) != 1 || got.Auditoria[0].Entidad != "asignacion" {
		t.Errorf("auditoria mal compuesta: %+v", got.Auditoria)
	}
	if got.GeneradoEn.IsZero() {
		t.Error("generado_en no debe ser cero")
	}
	// EXACTAMENTE una llamada al rastro por descarga exitosa, con el viaje correcto.
	if fakeOp.registrarLlamado != 1 {
		t.Fatalf("RegistrarDescargaViaje se llamo %d veces, se esperaba exactamente 1", fakeOp.registrarLlamado)
	}
	if fakeOp.registrarRecibio != id {
		t.Errorf("se registro descarga del viaje %v, se esperaba %v", fakeOp.registrarRecibio, id)
	}
}

// TestDescargarViajeAgotaPaginasDeBitacoraYAuditoria verifica que la composicion
// agota el cursor: con dos paginas de bitacora y dos de auditoria, el expediente
// trae todas las entradas, no solo la primera pagina.
func TestDescargarViajeAgotaPaginasDeBitacoraYAuditoria(t *testing.T) {
	id := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	sig := "cursor-2"
	fakeOp := &fakeOperacion{
		detalle: detalleDescarga(id),
		bitacoraPaginas: []*opclient.PaginaBitacora{
			{Items: []opclient.EntradaBitacora{{Id: uuid.New(), ViajeId: id, Texto: "e1", Ts: time.Now().UTC()}}, CursorSiguiente: &sig},
			{Items: []opclient.EntradaBitacora{{Id: uuid.New(), ViajeId: id, Texto: "e2", Ts: time.Now().UTC()}}},
		},
		auditoriaPaginas: []*opclient.PaginaAuditoria{
			{Items: []opclient.RegistroAuditoria{{Actor: "a1", Entidad: "viajes", EntidadId: uuid.New(), Ts: time.Now().UTC()}}, CursorSiguiente: &sig},
			{Items: []opclient.RegistroAuditoria{{Actor: "a2", Entidad: "viajes", EntidadId: uuid.New(), Ts: time.Now().UTC()}}},
		},
		registrarViaje: &opclient.Viaje{Id: id},
	}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "op-1", Audiencia: auth.AudienciaInterno})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.DescargaViaje
	decodificarCuerpo(t, rec, &got)
	if len(got.Bitacora) != 2 {
		t.Errorf("bitacora = %d entradas, se esperaban 2 (dos paginas agotadas)", len(got.Bitacora))
	}
	if len(got.Auditoria) != 2 {
		t.Errorf("auditoria = %d registros, se esperaban 2 (dos paginas agotadas)", len(got.Auditoria))
	}
}

// TestDescargarViajeFallaAlRegistrarNoEntregaExpediente: si operacion falla al
// dejar el rastro, la descarga no se entrega (no se sirve un expediente cuya
// descarga no quedo auditada). bff no tiene BD: el rastro es responsabilidad de
// operacion.
func TestDescargarViajeFallaAlRegistrarNoEntregaExpediente(t *testing.T) {
	id := uuid.MustParse("aaaa0000-0000-0000-0000-000000000000")
	fakeOp := &fakeOperacion{
		detalle:      detalleDescarga(id),
		registrarErr: platformerrors.NewDegradado("operacion no pudo escribir el rastro"),
	}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "op-1", Audiencia: auth.AudienciaInterno})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, se esperaba 503 (degradado al registrar)", rec.Code)
	}
	if fakeOp.registrarLlamado != 1 {
		t.Errorf("RegistrarDescargaViaje se llamo %d veces, se esperaba 1", fakeOp.registrarLlamado)
	}
}

// --- Exclusiones duras (DEC-009/DEC-039) ---

// TestDescargarViajeNoFiltraDatosExcluidos verifica a nivel HTTP que el cuerpo
// del expediente NUNCA contiene "cuenta espejo" ni "clave_amago": no existen en
// el modelo V1 ni en el contrato, asi que no hay representacion para ellos. Aun
// si operacion devolviera basura en campos de texto (autor/texto/entidad), el
// expediente solo serializa los campos contratados; estas marcas no pertenecen a
// ninguno.
func TestDescargarViajeNoFiltraDatosExcluidos(t *testing.T) {
	id := uuid.MustParse("bbbb0000-0000-0000-0000-000000000000")
	fakeOp := &fakeOperacion{
		detalle:        detalleDescarga(id),
		registrarViaje: &opclient.Viaje{Id: id},
	}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.DescargarViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "op-1", Audiencia: auth.AudienciaInterno})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	cuerpo := strings.ToLower(rec.Body.String())
	for _, excluido := range []string{"cuenta espejo", "cuenta_espejo", "espejo", "clave_amago", "clave amago", "amago"} {
		if strings.Contains(cuerpo, excluido) {
			t.Errorf("el expediente contiene el dato excluido %q (DEC-009/DEC-039): %s", excluido, rec.Body.String())
		}
	}
}

// platformErrProhibido devuelve el error de 403 aguas abajo (lo que produce el
// adaptador al traducir un 403 de operacion con platformerrors.NewProhibido).
// Simula el rechazo por alcance de operacion sin acoplar el test al adaptador HTTP.
func platformErrProhibido() error {
	return platformerrors.NewProhibido("operacion: fuera de alcance")
}
