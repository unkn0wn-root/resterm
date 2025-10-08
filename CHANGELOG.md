# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

### [0.1.8](https://github.com/unkn0wn-root/resterm/compare/v0.1.7...v0.1.8) (2025-10-08)

### [0.1.7](https://github.com/unkn0wn-root/resterm/compare/v0.1.6...v0.1.7) (2025-10-07)


### Features

* add request separator color ([9ec619d](https://github.com/unkn0wn-root/resterm/commit/9ec619dba65db7facf32a28761fdc0a8cb8af703))
* editor metadata styling ([78f14dd](https://github.com/unkn0wn-root/resterm/commit/78f14dd6108e5e9a855a3ebb91f66b8349ce35e1))

### [0.1.6](https://github.com/unkn0wn-root/resterm/compare/v0.1.5...v0.1.6) (2025-10-07)


### Bug Fixes

* guard history pane so j/k works after switching focus ([0e8b78b](https://github.com/unkn0wn-root/resterm/commit/0e8b78b0cf455647f1b93148324907a5fec4084b))

### [0.1.5](https://github.com/unkn0wn-root/resterm/compare/v0.1.4...v0.1.5) (2025-10-06)

### [0.1.4](https://github.com/unkn0wn-root/resterm/compare/v0.1.3...v0.1.4) (2025-10-04)


### Bug Fixes

* strip ansi seq before applying styles ([4ef6368](https://github.com/unkn0wn-root/resterm/commit/4ef63684913d5822d87e8219852a1832cf162ec7))

### [0.1.3](https://github.com/unkn0wn-root/resterm/compare/v0.1.2...v0.1.3) (2025-10-04)


### Bug Fixes

* **ui:** surface body diffs when viewing headers ([41fbbe6](https://github.com/unkn0wn-root/resterm/commit/41fbbe6516cda2f187cd8891afb3d18383acf26c))

### [0.1.2](https://github.com/unkn0wn-root/resterm/compare/v0.1.1...v0.1.2) (2025-10-04)


### Features

* normalize diff inputs and remove noisy newline warnings ([6f559b5](https://github.com/unkn0wn-root/resterm/commit/6f559b58fa8014c363c193a81372e67438194432))

### [0.1.1](https://github.com/unkn0wn-root/resterm/compare/v0.1.0...v0.1.1) (2025-10-04)


### Features

* add split for response, diff for requests and 'x' now deletes at mark ([786f121](https://github.com/unkn0wn-root/resterm/commit/786f1214a6d1169bed92f2f1020c42612080f16b))

## [0.1.0](https://github.com/unkn0wn-root/resterm/compare/v0.0.9...v0.1.0) (2025-10-04)


### Bug Fixes

* **editor:** normalize clipboard pastes and broaden delete motions ([c6af22c](https://github.com/unkn0wn-root/resterm/commit/c6af22c09f8a32a1e9d96dd6e2919f920e36d1f7))

### [0.0.9](https://github.com/unkn0wn-root/resterm/compare/v0.0.8...v0.0.9) (2025-10-03)


### Features

* add redo/undo, add new editor motions ([bcb1574](https://github.com/unkn0wn-root/resterm/commit/bcb1574bf6236a8e9f03fef05baf57dfed3c11f7))

### [0.0.8](https://github.com/unkn0wn-root/resterm/compare/v0.0.7...v0.0.8) (2025-10-02)


### Bug Fixes

* set the textarea viewport to refresh itself before clamping the scroll offset so non-zero view starts survive even when the viewport hasn’t rendered yet ([adccf37](https://github.com/unkn0wn-root/resterm/commit/adccf37972e202ee1868d7c152392c40360309e2))

### [0.0.7](https://github.com/unkn0wn-root/resterm/compare/v0.0.5...v0.0.7) (2025-10-02)


### Features

* add delete to be able to mark and delete section ([4766ff8](https://github.com/unkn0wn-root/resterm/commit/4766ff8e5fccabcb25d406d4a2a89d0009801c18))
* add undo to deleted buffer ([f600fea](https://github.com/unkn0wn-root/resterm/commit/f600fea8eedfa4c5632cb3b6867c386ad7172682))
* allow loading script blocks from external files ([1d33d6a](https://github.com/unkn0wn-root/resterm/commit/1d33d6ac52d588bdf947f2056416bba3b8a01017))
* respect the current viewport so we don't move editor to deleted line ([b5e11c1](https://github.com/unkn0wn-root/resterm/commit/b5e11c15fae62460c525f02e23cfd78a05fd5073))
* **ui:** add repeatable pane resizing chords and new "g" mode for resizing ([c261653](https://github.com/unkn0wn-root/resterm/commit/c26165306861bbab30b1a18a9889661b36e1c3d8))

### [0.0.6](https://github.com/unkn0wn-root/resterm/compare/v0.0.5...v0.0.6) (2025-10-01)


### Features

* allow loading [@script](https://github.com/script) blocks from external files ([9e9ff60](https://github.com/unkn0wn-root/resterm/commit/9e9ff60390b46bb9c850fd91e4c3bca94fc9d220))

### [0.0.5](https://github.com/unkn0wn-root/resterm/compare/v0.0.4...v0.0.5) (2025-10-01)

### [0.0.4](https://github.com/unkn0wn-root/resterm/compare/v0.0.3...v0.0.4) (2025-10-01)


### Features

* add saveAs for saving directly within editor ([78ef005](https://github.com/unkn0wn-root/resterm/commit/78ef005beac77c5f895d6d95fb4dc07fc008c08a))

### [0.0.3](https://github.com/unkn0wn-root/resterm/compare/v0.0.2...v0.0.3) (2025-10-01)


### Bug Fixes

* disable motions in insert mode ([7fd8985](https://github.com/unkn0wn-root/resterm/commit/7fd8985fd995741dabb0d657b289f0a5cf5208b0))
* inline request sending ([bde3f1f](https://github.com/unkn0wn-root/resterm/commit/bde3f1fc5b27486471cef25e6573cbe8ce1722cf))

### 0.0.2 (2025-10-01)


### Features

* add more vim motions to the editor ([a43a14c](https://github.com/unkn0wn-root/resterm/commit/a43a14c0973133a463e1564a5135f22bd318cf60))
* enable textarea selection highlighting in editor ([8ea748c](https://github.com/unkn0wn-root/resterm/commit/8ea748c185011c876481923c58bddfee360d345a))
* search ([b684ea8](https://github.com/unkn0wn-root/resterm/commit/b684ea839ec7b4efb2b99da221d569b2eece7a6d))


### Bug Fixes

* omit first event on search open ([b0b6f94](https://github.com/unkn0wn-root/resterm/commit/b0b6f94d56a688997024afbf96a05e8d0dcddb85))
