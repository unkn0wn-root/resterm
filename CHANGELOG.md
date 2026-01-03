# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

## [0.15.0](https://github.com/unkn0wn-root/resterm/compare/v0.14.2...v0.15.0) (2026-01-03)


### Features

* add tls to trace ([103e7cb](https://github.com/unkn0wn-root/resterm/commit/103e7cb25c09294c6ff968bd51b39c36462f45fb))

### [0.14.2](https://github.com/unkn0wn-root/resterm/compare/v0.14.1...v0.14.2) (2026-01-01)

### [0.14.1](https://github.com/unkn0wn-root/resterm/compare/v0.13.6...v0.14.1) (2025-12-30)

### [0.13.6](https://github.com/unkn0wn-root/resterm/compare/v0.13.5...v0.13.6) (2025-12-30)

### [0.13.5](https://github.com/unkn0wn-root/resterm/compare/v0.13.4...v0.13.5) (2025-12-29)


### Features

* add more methods/func coloring to rts editor ([b6ed022](https://github.com/unkn0wn-root/resterm/commit/b6ed022e9b911ea453d6e09240635fe1ff82fcc3))


### Bug Fixes

* don't expand file that has no children ([d865d5b](https://github.com/unkn0wn-root/resterm/commit/d865d5b6dd55972a1f423b0ad50cf15d77d1fd3b))
* when pressing 'l' on .rts file, set focus to the editor ([8b2ad88](https://github.com/unkn0wn-root/resterm/commit/8b2ad88b0326b8f595878a75b0feb7b0a3d9c8ae))

### [0.13.4](https://github.com/unkn0wn-root/resterm/compare/v0.13.3...v0.13.4) (2025-12-29)

### [0.13.3](https://github.com/unkn0wn-root/resterm/compare/v0.13.2...v0.13.3) (2025-12-29)

### [0.13.2](https://github.com/unkn0wn-root/resterm/compare/v0.13.1...v0.13.2) (2025-12-26)

### [0.13.1](https://github.com/unkn0wn-root/resterm/compare/v0.13.0...v0.13.1) (2025-12-26)

## [0.13.0](https://github.com/unkn0wn-root/resterm/compare/v0.12.0...v0.13.0) (2025-12-25)

## [0.12.0](https://github.com/unkn0wn-root/resterm/compare/v0.11.9...v0.12.0) (2025-12-19)


### Features

* rework minimize panes. Hide panes to make more room for other ([6e36871](https://github.com/unkn0wn-root/resterm/commit/6e36871264c031945b9526a00eafb9d0ab2c6b67))

### [0.11.9](https://github.com/unkn0wn-root/resterm/compare/v0.11.8...v0.11.9) (2025-12-19)


### Bug Fixes

* keep cursor based selection safe and sync navigator at EOF ([5dbb7df](https://github.com/unkn0wn-root/resterm/commit/5dbb7df491d4108f9695e1f67f952b8ae2b1785a))

### [0.11.8](https://github.com/unkn0wn-root/resterm/compare/v0.11.7...v0.11.8) (2025-12-19)


### Bug Fixes

* keep navigator state stable and restore cursor request fallback ([c20aac3](https://github.com/unkn0wn-root/resterm/commit/c20aac3a2b3590cc86d0674bc1e7f05060b38d11))

### [0.11.7](https://github.com/unkn0wn-root/resterm/compare/v0.11.6...v0.11.7) (2025-12-19)


### Features

* use reflow when generating hex/base64 and cache the results ([c2c8df9](https://github.com/unkn0wn-root/resterm/commit/c2c8df993fd3d59b2c71640c24701bf0c29506f5))


### Bug Fixes

* flickering on hex/base64 generation ([e00702f](https://github.com/unkn0wn-root/resterm/commit/e00702fd68f3ce0a1656ab21d8a6b82f7041c90c))
* reflow only for hex/base64 ([54f182f](https://github.com/unkn0wn-root/resterm/commit/54f182fcceffa857320af90d098d8e73cf68b8ce))

### [0.11.6](https://github.com/unkn0wn-root/resterm/compare/v0.11.5...v0.11.6) (2025-12-18)

### [0.11.5](https://github.com/unkn0wn-root/resterm/compare/v0.11.4...v0.11.5) (2025-12-18)


### Features

* async loading binary ([cad1dd3](https://github.com/unkn0wn-root/resterm/commit/cad1dd3dde1506b6ea1af2b6d0fe81bcbb4617da))
* cache hex/base64 bin response ([25cf7fa](https://github.com/unkn0wn-root/resterm/commit/25cf7fa4fc627362a51a0a714281a235872ca77b))
* defer large bin responses and add new shortcut to explicitly get whole body ([5c1e7d7](https://github.com/unkn0wn-root/resterm/commit/5c1e7d7ea06444ae99f52bbb76218a653fc8c507))

### [0.11.4](https://github.com/unkn0wn-root/resterm/compare/v0.11.3...v0.11.4) (2025-12-17)


### Bug Fixes

* **websocket:** timeout ([827abd7](https://github.com/unkn0wn-root/resterm/commit/827abd746e57aadc68dd76829d90bc42558f41f5))
* **websocket:** timeout via metadata ([3e1992a](https://github.com/unkn0wn-root/resterm/commit/3e1992afc21351a12455f8218c31dd07c7698ca0))

### [0.11.3](https://github.com/unkn0wn-root/resterm/compare/v0.11.2...v0.11.3) (2025-12-17)


### Bug Fixes

* **raw tab:** remove duplicate headers on empty response ([bcbb2cd](https://github.com/unkn0wn-root/resterm/commit/bcbb2cdd0cb3c4e3ee25304d9fc60e7be669a24f))

### [0.11.2](https://github.com/unkn0wn-root/resterm/compare/v0.11.1...v0.11.2) (2025-12-16)


### Bug Fixes

* add new bin response motions to help modal ([d78fdf9](https://github.com/unkn0wn-root/resterm/commit/d78fdf997756aa8355c9a200341c4a86859441db))

### [0.11.1](https://github.com/unkn0wn-root/resterm/compare/v0.11.0...v0.11.1) (2025-12-16)


### Features

* add gg/G jump up/down for each pane ([eba92c6](https://github.com/unkn0wn-root/resterm/commit/eba92c63784f7887aee0ce22d2954fcb758b4845))
* add quick jump (gg/G) in response tab ([e203244](https://github.com/unkn0wn-root/resterm/commit/e203244a2d894163f82c9086215fab47652bd410))
* input box for saving binary response ([d79d00a](https://github.com/unkn0wn-root/resterm/commit/d79d00af8098cefee750493300ecf91a5cb415ca))


### Bug Fixes

* keep grpc responses json friendly while exposing wire bytes ([f762bbe](https://github.com/unkn0wn-root/resterm/commit/f762bbe1582d7a4f6a781745695120987e5e934c))
* resync panes without clearing the whole terminal ([360f243](https://github.com/unkn0wn-root/resterm/commit/360f2434d4f311ef6dc11171db2f7d1094bb259f))

## [0.11.0](https://github.com/unkn0wn-root/resterm/compare/v0.10.5...v0.11.0) (2025-12-15)


### Features

* add file watcher to watch changes for opened file ([4b96268](https://github.com/unkn0wn-root/resterm/commit/4b962689efed8599a51f9c83376a79bc45343e7d))
* make extended file search opt-in and improve parsing secrets ([8dfe38b](https://github.com/unkn0wn-root/resterm/commit/8dfe38b7ae08c2a54ef5a440615f0c337409c475))


### Bug Fixes

* respect no fallback file lookup ([6cb7016](https://github.com/unkn0wn-root/resterm/commit/6cb701678dfceb96ef48e947777d14f83a9778a6))
* tests ([093074a](https://github.com/unkn0wn-root/resterm/commit/093074a6ad1cd11352c47fe609248ae2d92b5bac))
* **watcher:** show warn modal on local unsaved changes ([dfea2ad](https://github.com/unkn0wn-root/resterm/commit/dfea2ad093b3da8721d6e7927274918a98b71893))

### [0.10.5](https://github.com/unkn0wn-root/resterm/compare/v0.10.4...v0.10.5) (2025-12-12)


### Features

* save custom layout settings to persist after closing resterm ([5c1d41a](https://github.com/unkn0wn-root/resterm/commit/5c1d41a8a8f1a30353956e7c37499f6244470e47))

### [0.10.4](https://github.com/unkn0wn-root/resterm/compare/v0.10.3...v0.10.4) (2025-12-11)


### Features

* add 'type-writer' like scroll to the nav bar, editor and workflows ([510bba8](https://github.com/unkn0wn-root/resterm/commit/510bba88b20c73d204b619050724b40fdf6104e1))
* add jump to request shortcut and focus on request in editor while sending request ([7e0e0d6](https://github.com/unkn0wn-root/resterm/commit/7e0e0d658efd8263cbc02d37233b4ad3b39da7eb))


### Bug Fixes

* space should not bypass view mode ([33ffe45](https://github.com/unkn0wn-root/resterm/commit/33ffe458a0d4a300d9655a5b620de5bd0f02e938))

### [0.10.3](https://github.com/unkn0wn-root/resterm/compare/v0.10.2...v0.10.3) (2025-12-11)


### Bug Fixes

* display 1 script label ([e657615](https://github.com/unkn0wn-root/resterm/commit/e657615f5552f1d67a72451ba3f11cdc9db370e9))

### [0.10.2](https://github.com/unkn0wn-root/resterm/compare/v0.10.1...v0.10.2) (2025-12-10)

### [0.10.1](https://github.com/unkn0wn-root/resterm/compare/v0.10.0...v0.10.1) (2025-12-10)


### Bug Fixes

* don't expand at second right arrow press ([5a87ef0](https://github.com/unkn0wn-root/resterm/commit/5a87ef07c6bc97297b1fcaa378f76b076896b66b))

## [0.10.0](https://github.com/unkn0wn-root/resterm/compare/v0.9.5...v0.10.0) (2025-12-09)

### [0.9.5](https://github.com/unkn0wn-root/resterm/compare/v0.9.4...v0.9.5) (2025-12-07)


### Features

* abstract hints/autocomplete to ui pkg ([3ded126](https://github.com/unkn0wn-root/resterm/commit/3ded1265c621fc7ec39c937b224533a055c4c338))
* add editor hints to setting(s) ([a415db6](https://github.com/unkn0wn-root/resterm/commit/a415db64037e95ef54d73dc52d7d59b073920e62))
* add hints manager ([9fc3070](https://github.com/unkn0wn-root/resterm/commit/9fc3070d0cb5246d8b59a27d7f57f2c01b65318b))

### [0.9.4](https://github.com/unkn0wn-root/resterm/compare/v0.9.3...v0.9.4) (2025-12-06)


### Bug Fixes

* lint ([5b6ea7b](https://github.com/unkn0wn-root/resterm/commit/5b6ea7b76b643d6a8282055e92f547fbd32c800b))

### [0.9.3](https://github.com/unkn0wn-root/resterm/compare/v0.9.2...v0.9.3) (2025-12-06)


### Bug Fixes

* help overlay was swallowing the profile scheduler command ([675512b](https://github.com/unkn0wn-root/resterm/commit/675512bcadda1fc3d3c0aa61ac206327b32e13cc))

### [0.9.2](https://github.com/unkn0wn-root/resterm/compare/v0.9.1...v0.9.2) (2025-12-05)


### Features

* cancel pre-request scripts ([1aa2d19](https://github.com/unkn0wn-root/resterm/commit/1aa2d19e885403205586b15b00afb81979763f1c))


### Bug Fixes

* do not ovveride pinned pane ([c8c1ffb](https://github.com/unkn0wn-root/resterm/commit/c8c1ffb71984435a847bb82c8fba04bfbaba6601))
* pre-requests cancelation ([2d4464f](https://github.com/unkn0wn-root/resterm/commit/2d4464f661c3d3da68fc8877b76978f2e9cfe30e))
* resync response panes after building the summary ([91bb7a3](https://github.com/unkn0wn-root/resterm/commit/91bb7a39243f922b20034e28d54144ab4f7bb4c4))

### [0.9.1](https://github.com/unkn0wn-root/resterm/compare/v0.9.0...v0.9.1) (2025-12-04)

## [0.9.0](https://github.com/unkn0wn-root/resterm/compare/v0.8.5...v0.9.0) (2025-12-04)

### [0.8.5](https://github.com/unkn0wn-root/resterm/compare/v0.8.4...v0.8.5) (2025-12-01)


### Bug Fixes

* clear search in response tab ([1b87b10](https://github.com/unkn0wn-root/resterm/commit/1b87b105cb5d2bd2554559fba93a7087e066ecc7))
* search ([fcd2176](https://github.com/unkn0wn-root/resterm/commit/fcd2176afb55dc323556c71c19d8fc93adcc86b5))
* worflow search and better worflows navigation ([0f8ee5f](https://github.com/unkn0wn-root/resterm/commit/0f8ee5f8f02bc4c2e0ae932ba6845907efc40f0f))

### [0.8.4](https://github.com/unkn0wn-root/resterm/compare/v0.8.3...v0.8.4) (2025-11-29)


### Features

* prettify grpc response in workflows ([cefe457](https://github.com/unkn0wn-root/resterm/commit/cefe45733004751f2b63b1142af8c981350140ba))


### Bug Fixes

* avoid secret leaks in status/list labels and refresh on env change ([c32d638](https://github.com/unkn0wn-root/resterm/commit/c32d638c58e34708a826772b4fd87bed02305bb4))

### [0.8.3](https://github.com/unkn0wn-root/resterm/compare/v0.8.2...v0.8.3) (2025-11-28)


### Features

* improve workflow stats navigation and align gRPC display with HTTP formatting ([3b3dfe7](https://github.com/unkn0wn-root/resterm/commit/3b3dfe798e970d73e90eeebd34e046948e7e1b92))

### [0.8.2](https://github.com/unkn0wn-root/resterm/compare/v0.8.1...v0.8.2) (2025-11-27)


### Features

* add request-headers toggle, capture request metadata, and add scroll to workflows with j/k ([0255878](https://github.com/unkn0wn-root/resterm/commit/02558789597365a33112adcc2b0574c29e36ec15))

### [0.8.1](https://github.com/unkn0wn-root/resterm/compare/v0.8.0...v0.8.1) (2025-11-27)

## [0.8.0](https://github.com/unkn0wn-root/resterm/compare/v0.7.8...v0.8.0) (2025-11-23)


### Features

* wip - ssh ([9ccb01e](https://github.com/unkn0wn-root/resterm/commit/9ccb01e06eedf0e5a899e51129147f68037d679d))


### Bug Fixes

* don't double unlock and panic ([6baa5fd](https://github.com/unkn0wn-root/resterm/commit/6baa5fd55bf267d1237608e0013fb3fc6e288e19))
* grpc via ssh tunnel ([c00b4a4](https://github.com/unkn0wn-root/resterm/commit/c00b4a47f4fc8975b556f8cf48fa62a2f6034038))
* linter ([b73def9](https://github.com/unkn0wn-root/resterm/commit/b73def9d3038d8e14c64d4ed8469eb6128192e49))

### [0.7.8](https://github.com/unkn0wn-root/resterm/compare/v0.7.7...v0.7.8) (2025-11-21)

### [0.7.7](https://github.com/unkn0wn-root/resterm/compare/v0.7.6...v0.7.7) (2025-11-21)

### [0.7.6](https://github.com/unkn0wn-root/resterm/compare/v0.7.5...v0.7.6) (2025-11-20)

### [0.7.5](https://github.com/unkn0wn-root/resterm/compare/v0.7.4...v0.7.5) (2025-11-20)


### Bug Fixes

* profile report window, histogram styling and add profile param hints ([06d875e](https://github.com/unkn0wn-root/resterm/commit/06d875e2c7c8b3a5123706f636417ef93730447c))

### [0.7.4](https://github.com/unkn0wn-root/resterm/compare/v0.7.3...v0.7.4) (2025-11-19)


### Features

* add copy to clipboard shortcut for response ([246a765](https://github.com/unkn0wn-root/resterm/commit/246a7651be03982651135a95c25165cb221eb15b))

### [0.7.3](https://github.com/unkn0wn-root/resterm/compare/v0.7.2...v0.7.3) (2025-11-12)

### [0.7.2](https://github.com/unkn0wn-root/resterm/compare/v0.7.1...v0.7.2) (2025-11-12)


### Features

* read .env file with passing --env-file ([b97846a](https://github.com/unkn0wn-root/resterm/commit/b97846a5e8d775d07620eeb2931466e89eb3d650))

### [0.7.1](https://github.com/unkn0wn-root/resterm/compare/v0.7.0...v0.7.1) (2025-11-11)


### Features

* add configurable keyboard bindings + dynamic help overlay ([1d3c624](https://github.com/unkn0wn-root/resterm/commit/1d3c624e37e8adc1fc982c265cbf2b4df6ca417e))

## [0.7.0](https://github.com/unkn0wn-root/resterm/compare/v0.6.4...v0.7.0) (2025-11-09)


### Features

* **compare:** add multi environment diff workflow ([8928427](https://github.com/unkn0wn-root/resterm/commit/8928427232ed48451b7697bbe2260adb9bbdfb36))


### Bug Fixes

* linter ([0783256](https://github.com/unkn0wn-root/resterm/commit/078325644e88cf477aed96ae74ab6a4e448326a2))

### [0.6.4](https://github.com/unkn0wn-root/resterm/compare/v0.6.3...v0.6.4) (2025-11-06)


### Features

* **ui:** add pane minimization toggles and zoom handling ([81abf38](https://github.com/unkn0wn-root/resterm/commit/81abf3847313463956a6663f5c648a3ae1647f77))


### Bug Fixes

* powershell install script ([f5bd8ba](https://github.com/unkn0wn-root/resterm/commit/f5bd8bac874467ae3f42abbdea99f09f0156d3c3))
* remove old func and var after recent changes ([f829d48](https://github.com/unkn0wn-root/resterm/commit/f829d48e5ed873683a75d650336541f8cbc65baf))

### [0.6.3](https://github.com/unkn0wn-root/resterm/compare/v0.6.2...v0.6.3) (2025-11-01)

### Features

* add pane minimize toggles and zoom mode for sidebar/editor/response panes

### [0.6.2](https://github.com/unkn0wn-root/resterm/compare/v0.6.1...v0.6.2) (2025-11-01)


### Features

* improve response summary content-length rendering ([0ed32ab](https://github.com/unkn0wn-root/resterm/commit/0ed32ab5f5ef7591a811e64b5a2bf55987805518))

### [0.6.1](https://github.com/unkn0wn-root/resterm/compare/v0.6.0...v0.6.1) (2025-10-31)


### Features

* add content-length to the response sum ([596dc5d](https://github.com/unkn0wn-root/resterm/commit/596dc5d99fbe8ecb30cc0c332675b769706340aa))

## [0.6.0](https://github.com/unkn0wn-root/resterm/compare/v0.5.2...v0.6.0) (2025-10-26)


### Features

* add HTTP tracing, timeline UI, and OTEL export support ([af0cee1](https://github.com/unkn0wn-root/resterm/commit/af0cee1708e08b1ecbec577cf2a9365c0bc269d0))

### [0.5.2](https://github.com/unkn0wn-root/resterm/compare/v0.5.1...v0.5.2) (2025-10-24)


### Features

* add subcommands/hints to autocompleter ([9636b05](https://github.com/unkn0wn-root/resterm/commit/9636b05a5c635dccf14b51cec41a3a7c56837d58))

### [0.5.1](https://github.com/unkn0wn-root/resterm/compare/v0.5.0...v0.5.1) (2025-10-24)

## [0.5.0](https://github.com/unkn0wn-root/resterm/compare/v0.4.8...v0.5.0) (2025-10-23)


### Bug Fixes

* remove unused func and fix linter ([baf7c16](https://github.com/unkn0wn-root/resterm/commit/baf7c16f18097b68cbb335e99e6e4fe90d39b7d8))
* response pane width ([e877c17](https://github.com/unkn0wn-root/resterm/commit/e877c17735d9751a6aa79cebe96d531872dcd8b7))
* response pane width ([2673f39](https://github.com/unkn0wn-root/resterm/commit/2673f39ab7cbb12edaafc1d7faf8756af5934c7f))
* response pane width ([3c63b50](https://github.com/unkn0wn-root/resterm/commit/3c63b506012c1f2a3e50ec2698e180978ea39e90))

### [0.4.8](https://github.com/unkn0wn-root/resterm/compare/v0.4.7...v0.4.8) (2025-10-22)


### Features

* add OpenAPI -> resterm import pipeline ([11e76e2](https://github.com/unkn0wn-root/resterm/commit/11e76e2aaeb09f4e285299724154bd7fb2c7c796))

### [0.4.7](https://github.com/unkn0wn-root/resterm/compare/v0.4.6...v0.4.7) (2025-10-20)


### Features

* horizontal response pane ([#62](https://github.com/unkn0wn-root/resterm/issues/62)) ([9ae8f5d](https://github.com/unkn0wn-root/resterm/commit/9ae8f5dbff40fbb71b7977f1e1672f37a1100aef))
* new response wrapper ([3c46110](https://github.com/unkn0wn-root/resterm/commit/3c46110b788a39902c0b8b90d7dba3ff973e3bc0))

### [0.4.6](https://github.com/unkn0wn-root/resterm/compare/v0.4.5...v0.4.6) (2025-10-19)

### [0.4.5](https://github.com/unkn0wn-root/resterm/compare/v0.4.4...v0.4.5) (2025-10-18)


### Bug Fixes

* strips the CommandBar style’s left/right padding before styling and re-inserts it as plain spaces ([#58](https://github.com/unkn0wn-root/resterm/issues/58)) ([73bd114](https://github.com/unkn0wn-root/resterm/commit/73bd114f09d38ca89894f88ab19ebb2ff1f1b54f))

### [0.4.4](https://github.com/unkn0wn-root/resterm/compare/v0.4.3...v0.4.4) (2025-10-18)

### [0.4.3](https://github.com/unkn0wn-root/resterm/compare/v0.4.2...v0.4.3) (2025-10-18)


### Features

* add progress bar to updater ([#56](https://github.com/unkn0wn-root/resterm/issues/56)) ([cbd62df](https://github.com/unkn0wn-root/resterm/commit/cbd62dfb2be20e6e5d167c05684174311261a017))
* **editor:** add 8-column buffer scroll to margin ([#57](https://github.com/unkn0wn-root/resterm/issues/57)) ([560b3d6](https://github.com/unkn0wn-root/resterm/commit/560b3d6e7289b0873cbfdf41961fa45712eaecee))

### [0.4.2](https://github.com/unkn0wn-root/resterm/compare/v0.4.1...v0.4.2) (2025-10-18)


### Features

* stdout update changelog ([#55](https://github.com/unkn0wn-root/resterm/issues/55)) ([3a1f906](https://github.com/unkn0wn-root/resterm/commit/3a1f906d571b938522d2cb84bf38cc257ab49813))

### [0.4.1](https://github.com/unkn0wn-root/resterm/compare/v0.4.0...v0.4.1) (2025-10-18)


### Features

* resterm updater ([#54](https://github.com/unkn0wn-root/resterm/issues/54)) ([369955e](https://github.com/unkn0wn-root/resterm/commit/369955ecff0bca0a5fd310f9067174def95d6de7))

## [0.4.0](https://github.com/unkn0wn-root/resterm/compare/v0.3.1...v0.4.0) (2025-10-18)


### Features

* added custom themes ([1731cf3](https://github.com/unkn0wn-root/resterm/commit/1731cf32a01c64c356493369c03b5ce368a8d1de))
* faint/blur requests when in files ([49819f0](https://github.com/unkn0wn-root/resterm/commit/49819f0a3a14e045fe5648341b283a03218c7225))
* **ui/textarea:** add horizontal scroll and safe ANSI rendering ([3cd7151](https://github.com/unkn0wn-root/resterm/commit/3cd7151c10c94f0730246a4525b5d2f272820635))


### Bug Fixes

* lint and redundant code ([443cbe3](https://github.com/unkn0wn-root/resterm/commit/443cbe3e9a9046c319acb074f0d9a8283d12ba09))

### [0.3.1](https://github.com/unkn0wn-root/resterm/compare/v0.3.0...v0.3.1) (2025-10-16)


### Features

* add files/requests resize (g+h/l) ([501a2b2](https://github.com/unkn0wn-root/resterm/commit/501a2b2938de568b8039b69ce15bdfae0b603438))

## [0.3.0](https://github.com/unkn0wn-root/resterm/compare/v0.2.2...v0.3.0) (2025-10-16)


### Features

* add inline metadata hints (autocomplete) and make some small tweaks to editor ([658a9c2](https://github.com/unkn0wn-root/resterm/commit/658a9c2580f61b78e7693e4da3f8b80983bae449))


### Bug Fixes

* linter ([c9733c2](https://github.com/unkn0wn-root/resterm/commit/c9733c22402badccec47a5d0b9c11bc8b71888a2))

### [0.2.2](https://github.com/unkn0wn-root/resterm/compare/v0.2.1...v0.2.2) (2025-10-16)


### Features

* add version flag and auto install scripts for linux/mac and windows ([fc1ba4a](https://github.com/unkn0wn-root/resterm/commit/fc1ba4aefbc9a4d4cae24acab01b9f95b98199fa))

### [0.2.1](https://github.com/unkn0wn-root/resterm/compare/v0.2.0...v0.2.1) (2025-10-14)

## [0.2.0](https://github.com/unkn0wn-root/resterm/compare/v0.1.25...v0.2.0) (2025-10-13)


### Features

* new lexer for javascript objects to improve prettify ([1c43f8b](https://github.com/unkn0wn-root/resterm/commit/1c43f8b56ed66e591f582d4470471045e3ad28ec))
* **ui:** capture each request for workflow and assign to each workflow task ([5f96868](https://github.com/unkn0wn-root/resterm/commit/5f968687fcf4750a4a709d99075ef495c339969f))


### Bug Fixes

* allow both : / = and direct assigment for gobal/request/var ([4214875](https://github.com/unkn0wn-root/resterm/commit/42148751ba1c1841bf1e6698f965054c01920e63))

### [0.1.25](https://github.com/unkn0wn-root/resterm/compare/v0.1.24...v0.1.25) (2025-10-13)


### Features

* add workflow runner and colorize Stats ([8a6bdfb](https://github.com/unkn0wn-root/resterm/commit/8a6bdfb7b5a8df4a58cf4692a20b708621525789))
* **ui:** refine sidebar resizing and request list layout ([1cfb393](https://github.com/unkn0wn-root/resterm/commit/1cfb393cad7d1747a9253c606815678225e8c240))


### Bug Fixes

* made a mistake. Fix lint ([6e3cfd5](https://github.com/unkn0wn-root/resterm/commit/6e3cfd59c7b02fb659d1bdac331cabab6f194633))

### [0.1.24](https://github.com/unkn0wn-root/resterm/compare/v0.1.23...v0.1.24) (2025-10-11)


### Bug Fixes

* do not log profiler res if no-log is specified ([953be33](https://github.com/unkn0wn-root/resterm/commit/953be3394b0ec792b728152ac7c0dfb8580899a6))

### [0.1.23](https://github.com/unkn0wn-root/resterm/compare/v0.1.22...v0.1.23) (2025-10-11)


### Features

* **history:** record profile runs, add preview modal, enable delete ([bb90292](https://github.com/unkn0wn-root/resterm/commit/bb90292e2cb50fe7a40c7b53ce6e3333fad0dd37))


### Bug Fixes

* lint errors ([e11d388](https://github.com/unkn0wn-root/resterm/commit/e11d388038fada48c72ecb5e7551dda824d5b3c6))

### [0.1.22](https://github.com/unkn0wn-root/resterm/compare/v0.1.21...v0.1.22) (2025-10-10)


### Features

* added new meta  to profile request ([98a0a55](https://github.com/unkn0wn-root/resterm/commit/98a0a55f8055596c4c81b07e3606f7c916cbea46))

### [0.1.21](https://github.com/unkn0wn-root/resterm/compare/v0.1.20...v0.1.21) (2025-10-10)

### [0.1.20](https://github.com/unkn0wn-root/resterm/compare/v0.1.19...v0.1.20) (2025-10-10)


### Features

* add new focus on pane shortcuts ([6cf9cc7](https://github.com/unkn0wn-root/resterm/commit/6cf9cc748eff0248d40c9126635ed3179b3d1ecf))


### Bug Fixes

* suppress tab focus switching in insert mode ([1952a4f](https://github.com/unkn0wn-root/resterm/commit/1952a4f95e728d92508671b6cbfe414d86ef0830))

### [0.1.19](https://github.com/unkn0wn-root/resterm/compare/v0.1.18...v0.1.19) (2025-10-10)


### Features

* mask sensitive history data and decouple history replay from auto-send ([6139108](https://github.com/unkn0wn-root/resterm/commit/6139108d610e29e53beb53180d08765ff5aa2338))

### [0.1.18](https://github.com/unkn0wn-root/resterm/compare/v0.1.17...v0.1.18) (2025-10-10)

### [0.1.17](https://github.com/unkn0wn-root/resterm/compare/v0.1.16...v0.1.17) (2025-10-09)


### Features

* add new temp file ([3bcf158](https://github.com/unkn0wn-root/resterm/commit/3bcf158ec144e6745d410a2da3425ad900861507))
* temporary file and key bindings ([a31d297](https://github.com/unkn0wn-root/resterm/commit/a31d29707c1a4e2a71f4ec776b956290caa8d9e5))

### [0.1.16](https://github.com/unkn0wn-root/resterm/compare/v0.1.15...v0.1.16) (2025-10-09)


### Features

* shorten active header ([d351088](https://github.com/unkn0wn-root/resterm/commit/d35108846fc94afea84880f7fb6cab84e53ca45f))
* use description and tags in requests pane ([7ca1cfb](https://github.com/unkn0wn-root/resterm/commit/7ca1cfb3b0689370a661fd55a57d785d6a0ac784))


### Bug Fixes

* keep current scroll position while changing panes in response panes ([2767633](https://github.com/unkn0wn-root/resterm/commit/276763372340a055200afeb7affda158a954187a))
* sync editor with request selection after opening a req file ([8cfde0e](https://github.com/unkn0wn-root/resterm/commit/8cfde0e969ba330ba4c936b405d882e62baf3f73))

### [0.1.15](https://github.com/unkn0wn-root/resterm/compare/v0.1.14...v0.1.15) (2025-10-09)


### Features

* add oauth2 auth, globals, and capture support ([746d71e](https://github.com/unkn0wn-root/resterm/commit/746d71e8328df035e016c81f2f93680cd51748a0))
* persist [@capture](https://github.com/capture) file/request scopes and document usage ([ff2f9bb](https://github.com/unkn0wn-root/resterm/commit/ff2f9bbb18f659512fee10ab8201c589ad160771))

### [0.1.14](https://github.com/unkn0wn-root/resterm/compare/v0.1.13...v0.1.14) (2025-10-09)


### Features

* add backward search navigation and cover regex behavior ([9b26b64](https://github.com/unkn0wn-root/resterm/commit/9b26b64ecaa4603ef513ff964f626315887691a9))
* added search bar to the response tab ([64a7329](https://github.com/unkn0wn-root/resterm/commit/64a73294c7afcb8b487e27a49ab69dce261889d3))

### [0.1.13](https://github.com/unkn0wn-root/resterm/compare/v0.1.12...v0.1.13) (2025-10-09)


### Features

* **ui:** preserve raw indentation and harden ANSI stripping ([6985454](https://github.com/unkn0wn-root/resterm/commit/6985454f40d0699163d52d815e12dedafce09185))

### [0.1.12](https://github.com/unkn0wn-root/resterm/compare/v0.1.11...v0.1.12) (2025-10-08)


### Features

* add basic model_utils test (wrapper) ([a490629](https://github.com/unkn0wn-root/resterm/commit/a490629ab85e515c720c8dcd9c51876e28ac8ec3))
* add gh ci with lint, tests and build (on release only) ([7bfb06b](https://github.com/unkn0wn-root/resterm/commit/7bfb06b15130bdab2697ea4d2fbe81f5cf131eca))


### Bug Fixes

* graphql query builder from file ([fd12229](https://github.com/unkn0wn-root/resterm/commit/fd12229e0073717a189978fdaa733e1675f920d7))
* wrapToWidth to handle indentation better ([1514674](https://github.com/unkn0wn-root/resterm/commit/1514674240b8c72783c13823356649d6014bc66e))

### [0.1.11](https://github.com/unkn0wn-root/resterm/compare/v0.1.10...v0.1.11) (2025-10-08)

### [0.1.10](https://github.com/unkn0wn-root/resterm/compare/v0.1.9...v0.1.10) (2025-10-08)


### Bug Fixes

* **status:** utf-8 and truncation overflow for narrow width ([a5598cf](https://github.com/unkn0wn-root/resterm/commit/a5598cf045aa43f23585973f26a1620d5e58a1eb))

### [0.1.9](https://github.com/unkn0wn-root/resterm/compare/v0.1.8...v0.1.9) (2025-10-08)

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
