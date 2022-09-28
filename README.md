fil-fip36-vote-tally
============================

The code in this repository generates a reproducible dump of all [poll-relevant state](https://filpoll.io/poll/16) as of the poll-epoch for [FIP36](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0036.md) ([`2162760`](https://filscan.io/tipset/chain?height=2162760)).

The generated SQLite database contains all Deals, all SpActors, all MultiSigs, and all plain Accounts
which should be sufficient to tally the [votes as present in the live log](https://api.filpoll.io/api/polls/16/view-votes).

**No filtering** has been applied whatsoever: you will need to exclude disqualified/inactive entries yourself. The only modification was dropping the `f0` prefix from all ID addresses and representing them as actual integers, in order to save significant amounts of space. For the same reason the database contains no indexes: it is strongly recommended to add some before proceeding.

You can download the (compressed) result at: [ipfs://bafybeifo6tnb46wgedl4ofmk6w22pjk6kub5dzsx2nvhkav4xbfiym6mn4/filstate_2162760.sqlite.zst](https://bafybeifo6tnb46wgedl4ofmk6w22pjk6kub5dzsx2nvhkav4xbfiym6mn4.ipfs.w3s.link/filstate_2162760.sqlite.zst)

All you need in order to reproduce this result is a chain+state export containing the height in question. Below
you can see the log of such a run, and a ballpark idea how much time and space you will need.

```
~/fil-fip36-vote-tally$ ls -alh data/ ; for i in 1 2 ; do time go run ./parsestate/ ; ls -alh data/filstate_2162760.sqlite; sha256sum data/filstate_2162760.sqlite ; done ; ls -alh data/
```
```
total 81G
drwxrwxr-x 2 ubuntu ubuntu 101 Sep 28 01:01 .
drwxrwxr-x 3 ubuntu ubuntu  26 Sep 27 11:10 ..
-rw-rw-r-- 1 ubuntu ubuntu   0 Sep 28 00:38 .keepdir
-rw-rw-r-- 1 ubuntu ubuntu 81G Sep 27 10:56 minimal_finality_stateroots_2163120_2022-09-15_00-00-00.car
```
```
2022/09/28 01:02:00 generating new index (slow!!!!) at data/minimal_finality_stateroots_2163120_2022-09-15_00-00-00.car.idx
Processed      deals: 7548232     accounts: 1306006     msigs: 18449     providers: 589458

real    13m47.385s
user    9m4.721s
sys     5m45.469s
```
```
-rw------- 1 ubuntu ubuntu 1.6G Sep 28 01:15 data/filstate_2162760.sqlite
05fdb3e3015e355a0930b0d372b8ea83c271a61384eff9a9d44776baf5363190  data/filstate_2162760.sqlite
```
```
Processed      deals: 7548232     accounts: 1306006     msigs: 18449     providers: 589458

real    7m15.182s
user    5m5.571s
sys     2m28.363s
```
```
-rw------- 1 ubuntu ubuntu 1.6G Sep 28 01:23 data/filstate_2162760.sqlite
05fdb3e3015e355a0930b0d372b8ea83c271a61384eff9a9d44776baf5363190  data/filstate_2162760.sqlite
```
```
total 84G
drwxrwxr-x 2 ubuntu ubuntu  211 Sep 28 01:23 .
drwxrwxr-x 3 ubuntu ubuntu   26 Sep 27 11:10 ..
-rw------- 1 ubuntu ubuntu 1.6G Sep 28 01:23 filstate_2162760.sqlite
-rw-rw-r-- 1 ubuntu ubuntu    0 Sep 28 00:38 .keepdir
-rw-rw-r-- 1 ubuntu ubuntu  81G Sep 27 10:56 minimal_finality_stateroots_2163120_2022-09-15_00-00-00.car
-rw-rw-r-- 1 ubuntu ubuntu 1.9G Sep 28 01:08 minimal_finality_stateroots_2163120_2022-09-15_00-00-00.car.idx
```



## Lead Maintainer
[Peter 'ribasushi' Rabbitson](https://github.com/ribasushi)

## License
[SPDX-License-Identifier: Apache-2.0 OR MIT](LICENSE.md)
