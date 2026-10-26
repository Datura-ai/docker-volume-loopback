# Change Log

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
