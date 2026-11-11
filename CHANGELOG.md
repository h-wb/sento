# Changelog

## [0.8.0](https://github.com/h-wb/sento/compare/v0.7.0...v0.8.0) (2026-11-11)


### Features

* **files:** correct what a file is, and work on several at once ([#92](https://github.com/h-wb/sento/issues/92)) ([b5d9492](https://github.com/h-wb/sento/commit/b5d94923fd05c9fe5bf70e7c2cfaf9f14c0229e0))
* **media:** a naming page in three parts, with setups for Kavita and Komga ([#97](https://github.com/h-wb/sento/issues/97)) ([3bfb384](https://github.com/h-wb/sento/commit/3bfb3840718eb35943e292b768b3045b7c1ef986))
* **web:** a design pass over editions, files and the shell ([#95](https://github.com/h-wb/sento/issues/95)) ([31f84f4](https://github.com/h-wb/sento/commit/31f84f4183a0697fd5da719b15073546b7649511))
* **web:** naming and organize follow-through ([#85](https://github.com/h-wb/sento/issues/85)) ([2094d46](https://github.com/h-wb/sento/commit/2094d46bb9fe8055c343ec9bd163b71bfada19e1))


### Bug Fixes

* **library:** organize converges in one run ([#82](https://github.com/h-wb/sento/issues/82)) ([864cd57](https://github.com/h-wb/sento/commit/864cd57abc9b571f5b6173b120e01dc30f3d6304))
* **scan:** a parser re-read keeps a file's release link and claims ([#84](https://github.com/h-wb/sento/issues/84)) ([de9a382](https://github.com/h-wb/sento/commit/de9a382d36be4b9df2919aacee39d6c65a242305))
* **scan:** re-derive facts on reviewed files too ([#81](https://github.com/h-wb/sento/issues/81)) ([22d854d](https://github.com/h-wb/sento/commit/22d854d5720e74619ec6c3098aaf036c47f1bd20))
* **scan:** re-read the files 0.7.0 stamped without reading ([#91](https://github.com/h-wb/sento/issues/91)) ([5760161](https://github.com/h-wb/sento/commit/5760161300cd416d49f3265bf0bcee1476a5b5e7))
* **web:** say card where a card is meant ([#93](https://github.com/h-wb/sento/issues/93)) ([d0fb8ca](https://github.com/h-wb/sento/commit/d0fb8ca26db0b30a4c5c6d70a1e5af5159779b7e))

## [0.7.0](https://github.com/h-wb/sento/compare/v0.6.0...v0.7.0) (2026-11-09)


### Features

* **library:** an edition can show as its own series in readers ([#64](https://github.com/h-wb/sento/issues/64)) ([ea26648](https://github.com/h-wb/sento/commit/ea2664836e1d3055bc62a5eced115837a6e82387))
* **library:** name the collected version in ComicInfo Series ([#62](https://github.com/h-wb/sento/issues/62)) ([2b6cb3f](https://github.com/h-wb/sento/commit/2b6cb3f29506fb2050c899c8a40adc2514cb98c2))
* **library:** presented reader fields name a copy in readers ([#71](https://github.com/h-wb/sento/issues/71)) ([7bf60a0](https://github.com/h-wb/sento/commit/7bf60a0abac39cbeadc970768fa6dea5bc8d946b))
* **library:** version in the series folder template — one top folder per collected version ([#57](https://github.com/h-wb/sento/issues/57)) ([dd165d8](https://github.com/h-wb/sento/commit/dd165d89f853dbb357f426aa81d10edc1288d9d2))
* **library:** version token in file names — readers that parse file names see two series ([#61](https://github.com/h-wb/sento/issues/61)) ([a08898a](https://github.com/h-wb/sento/commit/a08898aeca2d6e8e46554a018b829697baf09c69))
* **scan:** re-derive filename facts when the parser version moves on ([#80](https://github.com/h-wb/sento/issues/80)) ([133c486](https://github.com/h-wb/sento/commit/133c486d38e5140a333b31cc83d365b789a023c2))
* **web:** edition reader name as a dropdown over the shared template input ([#74](https://github.com/h-wb/sento/issues/74)) ([f711a73](https://github.com/h-wb/sento/commit/f711a734e86626a7233e099729247c091ca592e2))


### Bug Fixes

* **deps:** update go dependencies ([#65](https://github.com/h-wb/sento/issues/65)) ([8480542](https://github.com/h-wb/sento/commit/84805429a140f956f2113d8ea12b007049b59769))
* **importer:** record every file of a pack when one waits for review ([#72](https://github.com/h-wb/sento/issues/72)) ([510f3b2](https://github.com/h-wb/sento/commit/510f3b292ebbb539e8a1185518194569d91489c9))
* **importer:** skip recorded pack files when a parked job is retried ([#73](https://github.com/h-wb/sento/issues/73)) ([5992202](https://github.com/h-wb/sento/commit/5992202576dae6f7f97d5f15aaff614d0876fa84))
* **library:** a volume book carries Volume only in ComicInfo ([#67](https://github.com/h-wb/sento/issues/67)) ([97cece3](https://github.com/h-wb/sento/commit/97cece3b930e6ea00967a59f0cfc7d07450c1164))
* **library:** an own-series-only edition patch applied nothing ([#66](https://github.com/h-wb/sento/issues/66)) ([dae3920](https://github.com/h-wb/sento/commit/dae3920ab16371f129115a7648e07757a16d9253))
* **parser:** a group name containing a color word no longer claims color ([#75](https://github.com/h-wb/sento/issues/75)) ([edc5620](https://github.com/h-wb/sento/commit/edc5620d2ae67516b2885af86daf2224f994668f))
* **review:** match whole release can re-map a pack's unconfirmed import claims ([#77](https://github.com/h-wb/sento/issues/77)) ([27c707d](https://github.com/h-wb/sento/commit/27c707d845decc054df9913f4f3952e780535af5))
* **store:** check presence against the folder a file lives in ([#63](https://github.com/h-wb/sento/issues/63)) ([032802d](https://github.com/h-wb/sento/commit/032802d681e0ac9ed7b48ec00d7c007c167c2906))
* **store:** judge presence by the edition folder, not a nested book folder ([#68](https://github.com/h-wb/sento/issues/68)) ([d563aac](https://github.com/h-wb/sento/commit/d563aacb4b7bff15f15f1a2348916d1d747e94aa))


### Performance Improvements

* **library:** batch organize planning and lift the 120s cap on per-series organize ([#58](https://github.com/h-wb/sento/issues/58)) ([065b0d3](https://github.com/h-wb/sento/commit/065b0d342ec1b3cbaa116e069c0b2d89afa1f32d))
* **review:** match dialogs load fast and open decluttered ([#76](https://github.com/h-wb/sento/issues/76)) ([0c3fec5](https://github.com/h-wb/sento/commit/0c3fec596966079f024984fa98e792d96ee3ca41))

## [0.6.0](https://github.com/h-wb/sento/compare/v0.5.0...v0.6.0) (2026-11-09)


### Features

* **library:** bulk organize all series with dry-run preview (Sonarr-style mass rename) ([#55](https://github.com/h-wb/sento/issues/55)) ([10ce918](https://github.com/h-wb/sento/commit/10ce918dc1a44b54eb8b531a839c0816a37f8860))
* **library:** per-edition top-level series folders ([#53](https://github.com/h-wb/sento/issues/53)) ([677cd88](https://github.com/h-wb/sento/commit/677cd88a476e31ab460e31cd6464ca8d74066f15))

## [0.5.0](https://github.com/h-wb/sento/compare/v0.4.1...v0.5.0) (2026-11-07)


### Features

* **library:** relocate stored paths when the library root changes ([#46](https://github.com/h-wb/sento/issues/46)) ([804cfa6](https://github.com/h-wb/sento/commit/804cfa6f51f80bea8c031e5b7284e1d6b76fbe3b))


### Bug Fixes

* **deps:** update go dependencies ([#40](https://github.com/h-wb/sento/issues/40)) ([f06c1e9](https://github.com/h-wb/sento/commit/f06c1e94a38ce7f7d92f3f905ec631938534950a))
* **review:** keep signature trust stable between preview and apply ([#44](https://github.com/h-wb/sento/issues/44)) ([3ad1bff](https://github.com/h-wb/sento/commit/3ad1bff59abf717315e769cdd907136ac3829bed))

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
