# Product

<!-- impeccable:product-schema 1 -->

This record summarizes the user's approved specification-tag plan and repository
product constraints. AGENTS.md and docs/ARCHITECTURE.md remain authoritative.

## Platform

web

## Users

People recording their own physical possessions and their lifecycle. Owners and
editors maintain the catalog; viewers can inspect authorized data without editing.

## Product Purpose

AssetLoop 物迹 connects a physical item's identity, descriptive configuration,
3D appearance and append-only financial history. The cost dashboard answers how
much an item cost, how long it has been held and its average daily holding cost.

## Operating Context

The current development workflow uses one local Go application, SQLite, server
templates and JavaScript. PostgreSQL is the supported SaaS database. Local and
OSS blobs share storage-neutral keys. Development, UAT and production are separate
approval gates; this feature is not a production release.

## Capabilities and Constraints

- Category and product model remain entities. Assets belong directly to a model.
- Optional typed tags describe configurations; the model bounds available values.
  This is not a commerce SKU validator and does not enforce sold combinations.
- A user's Harness owns product knowledge and model discovery. Resource candidates
  require explicit confirmation before becoming stable appearance defaults.
- Item model overrides precede confirmed appearance rules and generic model defaults.
- Financial events are append-only; corrections void and replace rather than overwrite.
- Market search and price time series are future work, not current capabilities.

## Accessibility & Inclusion

The approved interface supports Chinese and English, light/dark themes, user accent
colors, narrow screens, keyboard controls and reduced-motion preferences.
