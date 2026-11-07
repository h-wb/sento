# Changelog

## [0.4.1](https://github.com/h-wb/sento/compare/v0.4.0...v0.4.1) (2026-11-07)


### Performance Improvements

* **files:** fingerprint archives only when their stat signature changes ([#36](https://github.com/h-wb/sento/issues/36)) ([53e6e2f](https://github.com/h-wb/sento/commit/53e6e2f67758c50a3e7a6f0e0cdb20d23be91203))

## [0.4.0](https://github.com/h-wb/sento/compare/v0.3.0...v0.4.0) (2026-11-06)


### Features

* **files:** show current profile scores and format breakdowns ([#30](https://github.com/h-wb/sento/issues/30)) ([eef8ae7](https://github.com/h-wb/sento/commit/eef8ae78714d3db8e20ca6934be9ac41b8ab4c57))


### Bug Fixes

* **review:** preserve existing copies when reviewing packs ([#33](https://github.com/h-wb/sento/issues/33)) ([7f16e9f](https://github.com/h-wb/sento/commit/7f16e9f9fe7857314b5d1898c194c454bb2bc339))
* **review:** remove unsupported Kritika comment setting ([#34](https://github.com/h-wb/sento/issues/34)) ([8b211e3](https://github.com/h-wb/sento/commit/8b211e3bfc7e3a74b86eb64c6a032863c948025c))

## [0.3.0](https://github.com/h-wb/sento/compare/v0.2.0...v0.3.0) (2026-11-03)


### Features

* **acquisition:** add release unblock controls ([#28](https://github.com/h-wb/sento/issues/28)) ([0bfd120](https://github.com/h-wb/sento/commit/0bfd120417ae7d1ca0ff63076779399fee9a8c14))


### Bug Fixes

* **parser:** parse resolution tags before release groups ([#29](https://github.com/h-wb/sento/issues/29)) ([67aec23](https://github.com/h-wb/sento/commit/67aec230ed848888ec6391b49db2ffe3007ec064))
* **wanted:** handle searches that queue no releases ([#26](https://github.com/h-wb/sento/issues/26)) ([41807f1](https://github.com/h-wb/sento/commit/41807f11643ab727c1c1882b89acd40e6e8307d9))

## [0.2.0](https://github.com/h-wb/sento/compare/v0.1.1...v0.2.0) (2026-11-02)


### Features

* add recoverable file cleanup and archive unavailable releases ([#8](https://github.com/h-wb/sento/issues/8)) ([0e6bb78](https://github.com/h-wb/sento/commit/0e6bb7825f32c7b07cda21de141aec315109e48d))
* **review:** decline a whole pack and match a release at once ([#12](https://github.com/h-wb/sento/issues/12)) ([08338cc](https://github.com/h-wb/sento/commit/08338cc5d89c4153c80c60095ba3f71a7d110855))


### Bug Fixes

* explain partial imports and classify activity errors ([#6](https://github.com/h-wb/sento/issues/6)) ([f07785d](https://github.com/h-wb/sento/commit/f07785daff6ee981295b4bacf55fd31fbd4e0594))
* harden file cleanup and bulk review decisions ([#25](https://github.com/h-wb/sento/issues/25)) ([c4b36b0](https://github.com/h-wb/sento/commit/c4b36b0016bbb6ae4362b60392f8010684169945))
* **review:** read a pack's numbers before trusting the download plan ([#11](https://github.com/h-wb/sento/issues/11)) ([0ae2418](https://github.com/h-wb/sento/commit/0ae24186ad4e5155dd018966fd5e7b7672a1932d))
* **search:** keep quality upgrades out of scheduled searches ([#16](https://github.com/h-wb/sento/issues/16)) ([c17a694](https://github.com/h-wb/sento/commit/c17a69412acc23d273384edfc63536ab4f341112))
* **upgrade:** back file scores with their release claims ([#18](https://github.com/h-wb/sento/issues/18)) ([4a9dc55](https://github.com/h-wb/sento/commit/4a9dc5564743f8abc8d4ef61c1e27716e46d2302))
* **web:** keep input font at 16px on touch screens to stop iOS focus zoom ([#20](https://github.com/h-wb/sento/issues/20)) ([3cca581](https://github.com/h-wb/sento/commit/3cca581d0ec027fa7bd1c9e46928d84c2b907a9b))

## [0.1.1](https://github.com/h-wb/sento/compare/v0.1.0...v0.1.1) (2026-11-02)


### Bug Fixes

* **deps:** update go dependencies ([#35](https://github.com/h-wb/sento/issues/35)) ([1e18659](https://github.com/h-wb/sento/commit/1e18659a71dcf248d801ec16e351d4f17f639143))
* diagnose empty feeds and share concurrent source requests ([#2](https://github.com/h-wb/sento/issues/2)) ([1b5c78f](https://github.com/h-wb/sento/commit/1b5c78f4d680aa3e78a4123a32716e898193366f))
* **release:** isolate cross-compilation from tool setup ([#39](https://github.com/h-wb/sento/issues/39)) ([715bb16](https://github.com/h-wb/sento/commit/715bb1670f87eff1029bde3473fffbeb957c8842))
* require evidence of pack units and edition numbering ([#5](https://github.com/h-wb/sento/issues/5)) ([50e4c2b](https://github.com/h-wb/sento/commit/50e4c2b2803a7f90697590ee1bd9e512821876c8))
* restore fresh builds and changelog links after migration ([#1](https://github.com/h-wb/sento/issues/1)) ([7977cae](https://github.com/h-wb/sento/commit/7977cae2631fac36a9907ae5dccfaef8f34fe59a))
* restore source discovery and retry archive range failures ([#3](https://github.com/h-wb/sento/issues/3)) ([459a316](https://github.com/h-wb/sento/commit/459a316e22e23ee2c197ac7121ad8c7281d9a938))
* scope file upgrades and placement to collected versions ([#4](https://github.com/h-wb/sento/issues/4)) ([dd758c0](https://github.com/h-wb/sento/commit/dd758c0283731bb6658434ffeb53100f73809041))

## 0.1.0 (2026-11-02)


### Features

* model editions, collected versions, and explicit file coverage ([bddba8a](https://github.com/h-wb/sento/commit/bddba8a3df0c56b253a45f2aa1a26c0998f9c5c7))
* prepare Sento for public releases ([3da1a97](https://github.com/h-wb/sento/commit/3da1a97c5451e6f02833a670c8d43170c9229539))


### Bug Fixes

* **store:** let Postgres generate the series id in the prune test ([b9dd8d9](https://github.com/h-wb/sento/commit/b9dd8d955fda4e85475494284a52dd2b0724dd91))
