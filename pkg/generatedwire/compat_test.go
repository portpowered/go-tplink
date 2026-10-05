package generatedwire_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/generatedwire"
)

func TestHistoricalGeneratedModelAliases(t *testing.T) {
	t.Parallel()

	var legacy generatedwire.LoginCloudRequest

	var canonical = legacy

	if reflect.TypeOf(legacy) != reflect.TypeOf(canonical) {
		t.Fatalf("legacy type %T differs from canonical type %T", legacy, canonical)
	}

	legacy.Method = generatedwire.Login
	if legacy.Method != dependencymodels.Login {
		t.Fatalf("legacy enum constant %q differs from canonical %q", legacy.Method, dependencymodels.Login)
	}
}
