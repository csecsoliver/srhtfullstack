# Calculate PGP key expiration dates

This is a small tool to help with adding a key expiration date to the
meta.sr.ht database. It will iterate over all keys that currently don't have an
expiration date set in the database, check if the key has one, and if so,
update the database accordingly. Running this script multiple times wastes some
CPU cycles, but is otherwise idempotent. Hence, it is also no problem if a run
is aborted for some reason. It can simply be restarted.

Build with `go build` and run the resulting executable on meta.sr.ht.
