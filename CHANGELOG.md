# Changelog

## [0.1.0-rc.3](https://github.com/boneskull/gh-stack/compare/v0.1.0-rc.2...v0.1.0-rc.3) (2026-02-01)


### Features

* **cmd:** add submit command for unified cascade, push, and PR workflow ([#11](https://github.com/boneskull/gh-stack/issues/11)) ([5d18fad](https://github.com/boneskull/gh-stack/commit/5d18fad87bfc77d7172c28111f8d16ebe7d89539))
* **cmd:** improve adopt command ergonomics ([#8](https://github.com/boneskull/gh-stack/issues/8)) ([10a604e](https://github.com/boneskull/gh-stack/commit/10a604e48b49af37fd11afd8a3ac13c6e06e8f79))
* handle squash-merged parent PRs with --onto rebase ([#10](https://github.com/boneskull/gh-stack/issues/10)) ([85a9cc5](https://github.com/boneskull/gh-stack/commit/85a9cc5ad18c13d578dd0183fb7bee4c2c4e8f28))


### Bug Fixes

* log display for first-level children ([c7cba03](https://github.com/boneskull/gh-stack/commit/c7cba03f62fed1949642989185687bbc85460a42))


### Miscellaneous Chores

* set next release to 0.1.0-rc.3 ([4ddf92d](https://github.com/boneskull/gh-stack/commit/4ddf92d2b52661c2d1d316db73936cce9b9ed895))

## [0.1.0-rc.2](https://github.com/boneskull/gh-stack/compare/v0.1.0-rc.1...v0.1.0-rc.2) (2026-01-27)


### Miscellaneous Chores

* **ci:** disable attestation ([6069681](https://github.com/boneskull/gh-stack/commit/6069681fa9a8324a280eecf127bf48525f0e4eed))

## [0.1.0-rc.1](https://github.com/boneskull/gh-stack/compare/v0.1.0-rc.0...v0.1.0-rc.1) (2026-01-26)


### Bug Fixes

* **ci:** try to fix release workflow ([bf86c42](https://github.com/boneskull/gh-stack/commit/bf86c427b05edc790941b86126a884e91a67d625))
* **release:** release 0.1.0-rc.1 ([108baac](https://github.com/boneskull/gh-stack/commit/108baaca44bc426de08c2176088843fbb42b142d))

## 0.1.0-rc.0 (2026-01-26)


### Features

* **cmd:** add abort command ([6311401](https://github.com/boneskull/gh-stack/commit/631140180ff8359c7deb4336a650c575ba94f853))
* **cmd:** add adopt command ([3ac8345](https://github.com/boneskull/gh-stack/commit/3ac8345974ba0d8db1be020c0558ee114a71d3ab))
* **cmd:** add cascade command ([c8d5ac9](https://github.com/boneskull/gh-stack/commit/c8d5ac9f587820342cc4eafd61a4ec0afcdfcbe5))
* **cmd:** add continue command ([d9ab43d](https://github.com/boneskull/gh-stack/commit/d9ab43d2a0473f98835dd43366617d520dd89345))
* **cmd:** add create command ([d709d01](https://github.com/boneskull/gh-stack/commit/d709d019de639b7f609aa90a62e353ed5d1a55e7))
* **cmd:** add init command ([11e2f69](https://github.com/boneskull/gh-stack/commit/11e2f693cba3b0a91520ade2ebe1b91edb5f7f96))
* **cmd:** add link command ([d2f31e6](https://github.com/boneskull/gh-stack/commit/d2f31e65933236293b90e68c6c48db2fd57e4a0e))
* **cmd:** add log command with tree visualization ([619c023](https://github.com/boneskull/gh-stack/commit/619c0230c30e6f075cdb2436abd99e6a549241aa))
* **cmd:** add orphan command ([902dd45](https://github.com/boneskull/gh-stack/commit/902dd4576c1bdff14ceda821fb9f885bfc83ac48))
* **cmd:** add pr command with GitHub integration ([9dcd6cf](https://github.com/boneskull/gh-stack/commit/9dcd6cfb83bcf0142828c22ad590f8e71ad181b9))
* **cmd:** add push command ([272214f](https://github.com/boneskull/gh-stack/commit/272214f9055842f9bca130dd383f60f41fe491a0))
* **cmd:** add sync command ([d5d652f](https://github.com/boneskull/gh-stack/commit/d5d652fce9f7b0297dd7b2c0a3021a44ed95262b))
* **cmd:** add unlink command ([3ffd9a0](https://github.com/boneskull/gh-stack/commit/3ffd9a004a36d5fe992d0b385a552a5ca1979874))
* **cmd:** add version flag ([6b6b123](https://github.com/boneskull/gh-stack/commit/6b6b1230c4599a9c8ed31e99d67a2b061bd237a4))
* **config:** add Config type with GetTrunk ([15b69ec](https://github.com/boneskull/gh-stack/commit/15b69ece536404107a58957fd3050d44ba44e212))
* **config:** add ListTrackedBranches ([394a614](https://github.com/boneskull/gh-stack/commit/394a614a9f5cf72bf396e63595c576b39c6c0ab7))
* **config:** add parent branch operations ([9693ab8](https://github.com/boneskull/gh-stack/commit/9693ab8fe33d026b19585a1cdb9e1f7685c8c1b1))
* **config:** add PR number operations ([9f71a45](https://github.com/boneskull/gh-stack/commit/9f71a45af05e24dd71727ca1f006aef44ac2945c))
* **config:** add SetTrunk ([c80429a](https://github.com/boneskull/gh-stack/commit/c80429a8439091c10f0318dd58f9d37b71b087db))
* **git:** add branch existence and creation ([6b889da](https://github.com/boneskull/gh-stack/commit/6b889dae73619803036561f7b84cffc2ace4c0b4))
* **git:** add Git type with CurrentBranch ([253f1bf](https://github.com/boneskull/gh-stack/commit/253f1bf6eb097978be5a6644bdf9f0b5a2e55863))
* **git:** add rebase operations ([12f20cb](https://github.com/boneskull/gh-stack/commit/12f20cb45ee2c8018bbda712409d1447d5df5541))
* **git:** add working tree status checks ([1e4c286](https://github.com/boneskull/gh-stack/commit/1e4c2861a75804e516bb45aeee57a0b02a917540))
* **github:** add CreateComment method ([eb96644](https://github.com/boneskull/gh-stack/commit/eb966449fa936695a03c747142cd44622e545215))
* **github:** add draft PR support ([c09b29a](https://github.com/boneskull/gh-stack/commit/c09b29a00c715469c2c8071931ef8dfcbfc68abf))
* **github:** add ListComments and UpdateComment methods ([891f333](https://github.com/boneskull/gh-stack/commit/891f3332638914ee91df4280000b0f9e30e698af))
* **github:** add stack comment generator ([f9748ec](https://github.com/boneskull/gh-stack/commit/f9748ece4a49f0dc43c72a291da7112f8c732530))
* **pr:** create drafts for stacked PRs and post comments ([bb144b2](https://github.com/boneskull/gh-stack/commit/bb144b2c4924b0a6073f67c5c9f0fc7772c689b9))
* **state:** add cascade state persistence ([0ee0d8e](https://github.com/boneskull/gh-stack/commit/0ee0d8eca4a610283922f1c4759185b61d4980c3))
* **sync:** update stack comments and manage draft status ([54e75a7](https://github.com/boneskull/gh-stack/commit/54e75a7fcb71f99ac619f61186f25a519aa86d91))
* **tree:** add traversal helpers ([ea3f67a](https://github.com/boneskull/gh-stack/commit/ea3f67a2bea00e57a5d7a9a1ad2216e8851c61f9))
* **tree:** add tree building from config ([c9f7b41](https://github.com/boneskull/gh-stack/commit/c9f7b41891acd7a63c84f1ad79c61daaef675bdc))


### Bug Fixes

* **ci:** trigger release workflow on main branch push ([21141bf](https://github.com/boneskull/gh-stack/commit/21141bfc1eec1ebb7c9464c375ef8877f76fb2b3))
* **ci:** upgrade to golangci-lint v2 for Go 1.25 support ([156b8df](https://github.com/boneskull/gh-stack/commit/156b8df3282140afe1424c0a6790fa03a7489d48))
* **lint:** resolve errcheck warnings, disable gosec ([c23fa81](https://github.com/boneskull/gh-stack/commit/c23fa8135d9e96e0bf42eb7cd6d5e5babc3095e6))
