# Changelog

## 0.1.0 (2026-11-27)


### ⚠ BREAKING CHANGES

* rename the plugin to plots ([#6](https://github.com/golem-mc/plots/issues/6))

### Features

* add debugallowunsafe, debugexec, debugloadtest and debugsavetest commands ([#18](https://github.com/golem-mc/plots/issues/18)) ([9167c5e](https://github.com/golem-mc/plots/commit/9167c5e4c51e71e40d6ad5f12a15ec95d31fd58f))
* add single plot areas and /plot debugimportworlds ([#20](https://github.com/golem-mc/plots/issues/20)) ([4a029d9](https://github.com/golem-mc/plots/commit/4a029d9989846942f29362bff89e8d1ea04493f4))
* **commands:** add /plot command framework and core plot management commands ([#7](https://github.com/golem-mc/plots/issues/7)) ([aadbc80](https://github.com/golem-mc/plots/commit/aadbc801573816d9a00b6b7dda12c5f4326f473f))
* **commands:** add area, cluster, rating, comment, schematic, backup, expiry, and condense tools ([#8](https://github.com/golem-mc/plots/issues/8)) ([15561e6](https://github.com/golem-mc/plots/commit/15561e68f3e9c510633e3fd639d22596cc1fbeca))
* **core:** add core plot model, flags, config, lang, and SQLite storage ([#2](https://github.com/golem-mc/plots/issues/2)) ([8b960ec](https://github.com/golem-mc/plots/commit/8b960ec73f758677b6b8d148b01243ff0e96d5ad))
* **generator:** add HybridPlotWorld generator, plot areas, and plugin lifecycle ([#4](https://github.com/golem-mc/plots/issues/4)) ([10aa686](https://github.com/golem-mc/plots/commit/10aa68664f83d52895bb4bfe71aac51a979209f4))
* play the plot music disc on enter and stop it on leave ([#17](https://github.com/golem-mc/plots/issues/17)) ([6e73952](https://github.com/golem-mc/plots/commit/6e73952b0bf7e7cf8b33d8a3603982541132c826))
* **protection:** add plot protection, flags, movement, and Vault economy ([#5](https://github.com/golem-mc/plots/issues/5)) ([ac6386d](https://github.com/golem-mc/plots/commit/ac6386d46e7e7359db8de07e4962232408d3d535))
* prune excess and road entities when plot chunks unload ([#19](https://github.com/golem-mc/plots/issues/19)) ([6c84a80](https://github.com/golem-mc/plots/commit/6c84a80486700f2174e71ef25a33fbd0d320883f))


### Bug Fixes

* **deps:** update module github.com/golem-mc/golem to v0.2.0 ([#24](https://github.com/golem-mc/plots/issues/24)) ([b89545d](https://github.com/golem-mc/plots/commit/b89545d89d8b11af70feed76aea7d6cb880e13e8))
* **deps:** update module modernc.org/sqlite to v1.63.0 ([#23](https://github.com/golem-mc/plots/issues/23)) ([5226953](https://github.com/golem-mc/plots/commit/52269535a9187d5f59b2b92f54dff5784f74de4f))
* **generator:** fix road schematic rotation, wall clamping, merged walls, and add column tests ([#9](https://github.com/golem-mc/plots/issues/9)) ([69e6407](https://github.com/golem-mc/plots/commit/69e6407459e04e48e78b48d705618f137c263624))
* place default plot home at the largest merged region's north side ([#22](https://github.com/golem-mc/plots/issues/22)) ([0cab5e0](https://github.com/golem-mc/plots/commit/0cab5e0bc70d861cfc09c88b7867b2d64cfe27d0))


### Code Refactoring

* rename the plugin to plots ([#6](https://github.com/golem-mc/plots/issues/6)) ([26484fc](https://github.com/golem-mc/plots/commit/26484fcafb50a920270aaf4d1ec15bf19f16c999))
