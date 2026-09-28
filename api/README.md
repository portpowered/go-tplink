# API schemas and generated models

`openapi.yaml` describes the TP-Link cloud wire contract and generates the
transport-facing models in `pkg/generatedwire`. These types represent the
request and response shapes at the cloud boundary.

`client-models.openapi.yaml` describes the public model projection and
generates types in `pkg/tplinkmodels`. Keep it separate when a public model is
normalized or otherwise differs from its wire representation. The public
package also contains handwritten behavior helpers and typed client errors;
those do not belong in generated output.

Run `make generate-api` after changing either schema. The generated files are
checked in and must not be edited by hand. CI regenerates them and checks that
the generated output matches the repository.
