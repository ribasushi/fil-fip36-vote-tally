fil-fip36-vote-tally
============================

The code in this repository generates a reproducible dump of all [poll-relevant state](https://filpoll.io/poll/16) as of the poll-epoch for [FIP36](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0036.md) ([`2162760`](https://filscan.io/tipset/chain?height=2162760)).

### Data summary

The generated SQLite database contains all Deals, all SpActors, all MultiSigs, and all plain Accounts, which in turn should be sufficient to tally [the votes, as present in the live log](https://api.filpoll.io/api/polls/16/view-votes).

The current version of this code produces a single-file standard SQLite database with SHA2-256 of `0d51f09d5cc015fae2838ca90dbe7800beb0968b90eb4da2ca2185742b548f49`. The process takes about ~8 minutes. You can download the (compressed) current result at: [ipfs://bafybeib3jcbsqmtjxcrafkgpldrsr3w4ubu4t6aqb5gyjhnpwhfd5r6viu/filstate_2162760.sqlite.zst](https://bafybeib3jcbsqmtjxcrafkgpldrsr3w4ubu4t6aqb5gyjhnpwhfd5r6viu.ipfs.w3s.link/filstate_2162760.sqlite.zst) . The count of processed entries is:

```
Processed      deals: 7548232     accounts: 1306006     msigs: 18449     providers: 589458
```

**No filtering** has been applied whatsoever: you will need to exclude disqualified/inactive entries yourself. The only modification was dropping the `f0` prefix from all ID addresses and representing them as actual integers, in order to save significant amounts of space. For the same reason the database contains no indexes: it is strongly recommended to add some before proceeding.

### Preliminary poll results

Having a database makes result polling really easy: [entire logic fits on a single page](https://github.com/ribasushi/fil-fip36-vote-tally/blob/b0833c04132/updatevotes/main.go#L249-L301)

<details><summary>Preliminary unofficial **NOT AUDITED** results of FilPoll 16</summary>

```
$ go run ./updatevotes/
2022/09/29 11:20:58 ignoring ballot {49 t1giru42mia2x27svolr3n7vth7byb77eshpc77iq 2022-09-28 18:12:09.179 +0000 UTC}: unknown robust address
2022/09/29 11:20:59 Processed 715 ACCEPT and 566 REJECT votes
2022/09/29 11:20:59 Calculating preliminary results ( takes about a minute )
2022/09/29 11:22:00

  Group: BalancesNfil
Abstain:  97.7%   587232862540689920
    Yea:  65.0%     8895619646577080
    Nay:  35.0%     4789430246299900

2022/09/29 11:22:00

  Group: DealBytesProvider
Abstain:  18.6%    39738054375265792
    Yea:  16.9%    29314317179453952
    Nay:  83.1%   144290514343871232

2022/09/29 11:22:00

  Group: DealBytesClient
Abstain:  62.7%   133795964267689216
    Yea:  19.2%    15266925616562176
    Nay:  80.8%    64279996014339584

2022/09/29 11:22:00

  Group: SpRawBytesMiB
Abstain:  63.6%       11776224067584
    Yea:  64.9%        4378047905792
    Nay:  35.1%        2369927970816
```
</details>

### Reproducibility

All you need in order to reproduce this result is a chain+state export containing the height in question. Below you can see the log of such a run, and a ballpark idea how much time and space you will need.

<details><summary>Example double-run of an earlier version at https://github.com/ribasushi/fil-fip36-vote-tally/commit/8ba5208ffd</summary>

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

</details>


## Lead Maintainer
[Peter 'ribasushi' Rabbitson](https://github.com/ribasushi)

## License
[SPDX-License-Identifier: Apache-2.0 OR MIT](LICENSE.md)
