# Repository instructions

- Keep this module usable without any consuming application or parent workspace.
- Keep TP-Link protocol behavior in `pkg/tplink` and public models and errors in `pkg/tplinkmodels`.
- Put account credentials on named requests; keep the reusable client safe for concurrent accounts.
- Keep wire formats, transport mechanics, and provider constants out of application adapters.
- Label fixtures as captured, synthetic, or historical. Fixtures without verifiable capture provenance are synthetic. Do not commit credentials or private captures.
- Run `make check` and `make lint` for changed code. Keep live vendor tests opt-in.
