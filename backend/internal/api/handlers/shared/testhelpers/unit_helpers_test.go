package testhelpers

import (
	"reflect"
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// AllShowsVisible must fill in EVERY function field on the mock.
//
// Its meaning is "this test is not about the gate", and the zero mock refuses
// everything, so a field left nil grants some entity types and silently refuses
// one more. The failure surfaces in an unrelated handler test as a product bug,
// which is the most expensive way to learn that a new entity type reached the
// interface.
//
// Reflection rather than a per-field assertion, so a method added to
// ShowVisibilityInterface and its generated mock is covered without anybody
// remembering to extend this.
func TestAllShowsVisibleFillsEveryGate(t *testing.T) {
	gate := AllShowsVisible()
	value := reflect.ValueOf(gate).Elem()
	valueType := value.Type()

	funcFields := 0
	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		if field.Type.Kind() != reflect.Func {
			continue
		}
		funcFields++
		if value.Field(i).IsNil() {
			t.Errorf("AllShowsVisible leaves %s nil, so the mock refuses that entity type "+
				"while granting the rest. Fill it in.", field.Name)
		}
	}

	// The sweep is only a guard while it finds fields. A renamed or removed field
	// set would otherwise pass vacuously.
	if funcFields == 0 {
		t.Fatal("MockShowVisibility has no function fields, so this test asserted nothing")
	}

	// The fields must also be wired to the methods that read them, which
	// reflection over field names cannot see.
	viewer := contracts.ShowViewer{}
	if !gate.ShowVisibleTo(1, viewer) {
		t.Error("AllShowsVisible refuses a show")
	}
	if !gate.CollectionVisibleTo(1, viewer) {
		t.Error("AllShowsVisible refuses a collection")
	}

	// The opt-out has to be an explicit call, so the ZERO mock must refuse. A
	// default-permissive mock would make every handler test that forgets the gate
	// pass.
	zero := &MockShowVisibility{}
	if zero.ShowVisibleTo(1, viewer) || zero.CollectionVisibleTo(1, viewer) {
		t.Error("the zero MockShowVisibility grants an entity, so a handler test that never " +
			"wired the gate would still pass")
	}
}
