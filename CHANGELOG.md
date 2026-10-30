# Change Log

## 1.0.0-lium9 - 2026-10-29

### Fixed

- UnMount keeps the final lease until the kernel mount is gone: a failed `findmnt` inspection or
  `umount` leaves the lease, so Docker's retry reaches the umount instead of being refused as a
  lease-less call while the filesystem stays mounted. A non-final lease is still dropped up front.

## 1.0.0-lium8 - 2026-10-29

### Fixed

- The driver's mutex is locked on the driver itself: the handlers had value receivers, so each
  request locked a copy and volume requests were never serialised.
- A loop device a plugin killed between `losetup --find --show` and `mount` left attached to the
  data file (no autoclear, no kernel mount) is detached: by Mount before it attaches, as
  `--nooverlap` would hand the stale device back; by UnMount when there is no kernel mount, together
  with the empty mount-point dir and the state dir; by Delete before it removes the image, which
  stays when the detach fails, since an unlinked image keeps its loop device alive and no later call
  can find it by path.
- A stray loop device is detached only after its backing file is verified through the open device
  (`LOOP_GET_STATUS64`, then `LOOP_CLR_FD` on the same descriptor): between the listing and a
  `losetup -d` by name the number can be freed and given to another image.
- The mount source at the mount point is read from `findmnt` stdout only; a warning on stderr made
  the volume's own mount read as foreign.

## 1.0.0-lium7 - 2026-10-29

### Fixed

- The loop device is read from `losetup --find --show` stdout only and must be a plain `/dev/loopN`:
  a warning losetup printed on stderr while succeeding used to be glued in front of the path, so the
  cleanup `losetup -d` and the `mount` received a multi-line string.

## 1.0.0-lium6 - 2026-10-27

### Fixed

- A loop device without direct I/O in effect (a backing filesystem that refuses O_DIRECT, or an
  unreadable `/sys/block/<loop>/loop/dio`) fails the Mount and is detached, instead of being handed
  out as a buffered volume with a warning: the host page cache this generation removes would be
  back behind it.

## 1.0.0-lium5 - 2026-10-27

### Fixed

- A mount already present at the volume's mount point is handed out only when it sits on a loop
  device of this volume's data file; anything else there fails the Mount instead of being served
  and later unmounted as this volume.
- The `dd` fallback writes the whole rounded size: the tail below one block is written in sectors,
  not truncated into a hole, so a regular (`sparse=false`) volume reserves all of its space.
- ext4 volumes are made with 4096-byte blocks; under 512 MB mkfs.ext4 picked 1024-byte blocks,
  which the 4096-byte-sector loop refuses.
- A failed autoclear mark after a successful mount is a warning, not a failed Mount: the mount was
  up without a lease and the device stayed attached until UnMount.
- Mount logs a warning when the loop device reports direct I/O off (a backing filesystem that
  refuses O_DIRECT gives a buffered loop behind the same command).

## 1.0.0-lium4 - 2026-10-27

### Fixed

- Mount attaches the image with `losetup --nooverlap`, so an image that already has a loop device
  gets that device back, as the implicit loop of `mount` gave it. Without it a Mount that did not
  see the first kernel mount (a restarted plugin whose mount view was lost) put a second
  filesystem instance on the same image.
- A data file made through the `dd` fallback (a data dir without `fallocate`) is also sized in whole
  4096-byte sectors: `dd` writes whole blocks and `truncate` adds the tail.

## 1.0.0-lium3 - 2026-10-27

### Fixed

- A volume whose size is not a multiple of 4096 bytes (the validator asks for `<n>g` = n×10⁹)
  could not be mounted on a host without the xfs module loaded: the loop device dropped the
  partial last sector, blkid failed on the read error there and `mount` fell through to
  `fuseblk`. Create now rounds the data file up to whole sectors and Mount passes the
  filesystem type explicitly.

## 1.0.0-lium2 - 2026-10-26

### Changed

- Mount attaches the image through `losetup --direct-io=on --sector-size 4096` instead of the
  implicit loop of `mount`: the host page cache no longer holds the image's unwritten data, so an
  overlay unmount on the host does not wait for the volume's writes to reach the disk. New XFS
  volumes are made with 4096-byte sectors to match; volumes made by earlier versions keep their
  512-byte sectors and do not mount on this version.
- Mount reuses an existing kernel mount and reconnects a lost one (lease files can outlive the
  mount after a plugin restart).

## 1.0.0-lium1 - 2026-10-09

### Changed

- Rebuilt on Go 1.26 / Alpine 3.24; XFS volumes are made with the 2019 feature set so every kernel
  in the fleet mounts them.

## 1.0 - 2019-02-13

### Added

- Initial implementation
