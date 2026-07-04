# Stale-first catalog refresh

Once sources are registered (PSY-1149), refresh the **stalest first** instead of by hand.

## Invoking

> `/ingest <env> — refresh the N stalest registered sources`

Agent runs: `ph sources stale --limit N` → matching ingest per row → `ph sources refresh` to stamp. Pause for OK at each dry-run.

## The loop

```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli
bun run src/entry.ts --env <env> sources stale --limit 20 --max-failures 5
```

For each row (`TYPE ID LAST-REFRESHED FAILS SOURCE-URL`):

1. **Run matching workflow** using `SOURCE URL`:
   - `TYPE=venue` → [venue-events.md](venue-events.md)
   - `TYPE=label` → [label-roster.md](label-roster.md)
   Dry-run + QA scans, OK, `--confirm` (idempotent).
2. **Stamp refresh:**
   ```bash
   bun run src/entry.ts --env <env> sources refresh <venue|label> <id>
   # on failure: POST /admin/sources/failure (no ph subcommand yet)
   ```

## Seeding the registry

```bash
bun run src/entry.ts --env <env> search venue "Empty Bottle"
bun run src/entry.ts --env <env> sources register venue <id> "https://www.emptybottle.com/"
bun run src/entry.ts --env <env> sources register label <id> "https://www.sacredbonesrecords.com/pages/artists"
```

Seed from venue registry ([venue-events.md](venue-events.md)) and label registry ([label-roster.md](label-roster.md)). Register is idempotent (upsert).

**Multi-room orgs:** one calendar URL → register **one row per venue entity** that gets shows; stamp each after refresh.

Stage seed (2026-06-21): First Avenue/94, Empty Bottle/14, Thalia Hall/107, Club Congress/109, Schubas/110, Lincoln Hall/111, Sleeping Village/112, Metro Baltimore/113, Zebulon/43, Sacred Bones label/1.

> `sources failure` is admin API only — use `curl` if needed.
