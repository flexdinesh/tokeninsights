# Browser fixtures

Browser setup starts one personal backend through `tokeninsights web --open=false`
using a private temporary home. Synthetic Pi records seed confirmed and excluded
usage. Setup waits for capture acceptance and published processing before tests.

Each run owns its fixture metadata, logs and browser artifacts. Teardown stops its
own child and removes its temporary storage.
