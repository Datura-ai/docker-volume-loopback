# Mount recovery fix

This branch starts at upstream `4d6079b7df8d2f5300e4e271a8b9571feb25618d` and retains the existing Lium build and XFS feature flags. It does not publish a plugin image or change an installed plugin.

## Failure and change

The old Mount handler treats an existing lease as an error but wraps a nil error. It returns an empty mount path without reporting a failure. It also assumes that any lease file proves the filesystem is mounted. Lease files can outlive the mount after a plugin restart.

Mount now checks the kernel mount table through `findmnt`, which is already included by the plugin's util-linux package. Repeated requests reuse the lease and return the actual mount path. If the kernel mount is missing, Mount reconnects the existing backing file. An empty stale mount directory can be reused; a nonempty directory causes an explicit error. Failure removes only a lease created by that request. It preserves earlier leases and never recursively deletes the mount directory.

Lease counting in UnMount and Delete is unchanged. No volume is recreated or reformatted during recovery.

## Verification

Run:

```sh
bash tests/run-mount-recovery.sh
```

Requires Go and a local Linux Docker engine with loop-device and filesystem support. The runner uses a disposable privileged container with networking disabled. It copies in the test executable and mounts only files created under the container's temporary test directories. It does not bind host directories or a Docker socket. Its container is removed on exit.

The tests cover XFS and ext4, repeated requests with the same lease, lost mounts with surviving leases, existing and missing mount directories, a new lease after restart, retained data, rejection of nonempty directories, cleanup of only the failed request's lease, and unmounting when the last lease ends. Unit tests require an explicit error if mount-state inspection is unavailable or ambiguous.

The old implementation failed all 11 original regression cases. The corrected implementation passed those cases plus the additional lease-cleanup, multi-client, and mount-state error cases.

The pinned upstream source has two pre-existing printf-vet errors in UnMount. The runner disables that separate vet step; it executes the tests. Runtime tests were run on Linux arm64 in local Docker. They simulate loss of the mount while preserving leases and reconstruct the manager. They do not establish native Docker 24 x86 daemon-crash recovery for a newly published artifact. That remains a release check.

## Delivery

Before enabling this change on executors, build a new plugin version, repeat the native daemon-restart check on a disposable machine, and pin its verified manifest in the validator. Existing plugins and renter volumes must not be upgraded or removed automatically.
