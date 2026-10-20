# Cleanup run artifacts

Reusable composite action built entirely from pinned third-party actions. There
are no custom scripts or shell steps:

- [octokit/request-action](https://github.com/octokit/request-action) reads the run
  status and artifact counts from GitHub's API.
- [joutvhu/delete-artifact](https://github.com/joutvhu/delete-artifact) lists and
  deletes the target run's artifacts, with pagination and all names selected.
- [nick-fields/assert-action](https://github.com/nick-fields/assert-action) requires
  other runs to be completed before deletion and a final artifact count of zero.

```yaml
permissions:
  contents: read
  actions: write
steps:
  - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
  - uses: ./.github/actions/cleanup-artifacts
    with:
      run-id: ${{ github.run_id }}
```

Use an Ubuntu runner and call this after every artifact producer and consumer
finishes. The Release workflow's cleanup job depends on validation, both build
matrices, and publication. Its `always()` condition attempts cleanup even when
those jobs fail, are skipped, or are cancelled, including manual verification runs.
The current run may still be active; other target runs must be completed.

Deletion is permanent, including diagnostic artifacts from failed runs. Release
assets, caches, and workflow logs are preserved. An empty run is a successful
no-op. Leftovers receive a second deletion attempt; the final API count determines
success even if the deletion action reports success after individual failures.
Counts use `total_count`, so verification covers more than one page. API errors
or remaining artifacts fail the job. The release cleanup job has a 15-minute timeout.

The upstream deletion action uses name-based deletion. Duplicate names from older
attempts or its pagination limit may require additional cleanup runs; verification
fails instead of reporting a partial cleanup as successful. GitHub outages,
disabled workflows, or force cancellation of the cleanup itself can prevent cleanup.
Storage accounting may take additional time to reflect successful deletion.
