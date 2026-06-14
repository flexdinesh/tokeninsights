# Rename Pi Package to @tokeninsights/pi

## Summary

Update the Pi extension package name to `@tokeninsights/pi` everywhere it is referenced in package metadata, scripts, runtime log prefixes, README, design docs, decision logs, and existing plans.

## Key implementation changes

- Change `packages/plugins/pi/package.json` package name to `@tokeninsights/pi`.
- Update root `package.json` `build:pi` pnpm filter to target `@tokeninsights/pi`.
- Update Pi install instructions in `README.md` to use `@tokeninsights/pi` in Pi settings.
- Update `docs/design.md` architecture/package reference to the new package name.
- Update Pi extension console error prefixes in `packages/plugins/pi/src/index.ts` to match the package name.
- Update tracked historical docs/plans that reference the legacy unscoped Pi package name so repository search results consistently point to `@tokeninsights/pi`.

## Tests or verification

- Search for the legacy unscoped Pi package name to confirm no remaining references.
- Run `pnpm run build:pi` to confirm the updated pnpm filter resolves and the Pi package still typechecks.

## Decisions made by the user

- User requested the Pi package name be changed to `@tokeninsights/pi`.
- User requested all references, relevant places, and docs be updated.
- User requested this plan be written to a file and executed.

## Tradeoffs and risks discussed

- Updating historical decision logs/plans keeps repository search results clean but makes archival records anachronistic. This was included because the user requested all references/docs and approved the execution plan that listed historical docs/plans.
- No schema or durable storage changes are involved.

## Remaining open questions

None.

## Execution guidance

If execution deviates from this plan, update this plan file to reflect the latest approved plan and surface the deviation to the user before continuing.
