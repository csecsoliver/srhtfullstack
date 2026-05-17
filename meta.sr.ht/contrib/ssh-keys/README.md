# Backfill structured SSH key fields

This is a small tool to help backfill the datbase fields for certain properties
of SSH keys that were added in meta.sr.ht 0.86.0. Running this script multiple
times is idempotent. It is also no problem if a run is aborted for some reason.
It can simply be restarted.

Build with `go build` (in this directory) and run the resulting executable on
meta.sr.ht (must have the same configuration as the service).
