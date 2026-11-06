# minimal-layer1-blockchain

Lab 01 for the Blockchain course: a minimal Layer 1 blockchain written in Go. The simulator creates 10 validators on an untrusted simulated network and runs Tendermint-style BFT consensus (Propose → Prevote → Precommit, a 2f+1 quorum, and round-based locking). It then finalizes blocks containing signed key–value transactions.

- **Cryptography:** Ed25519 and SHA-256. Signatures use domain separation: `TX:`, `HEADER:`, `PROPOSAL:`, and `VOTE:`, combined with the chain ID.
- **Simulated failures:** Before GST, the network can drop, duplicate, reorder, and delay messages. It also enforces rate limits and blocks peers that spam. Up to f = 3 validators may be Byzantine (crash, remain silent, equivocate, propose invalid blocks, or spam).
- **Full determinism:** Single-threaded execution, virtual time, and a seeded RNG. The same configuration and seed produce byte-for-byte identical logs and the same final state hash.

## Requirements

- Go ≥ 1.24. Only the standard library is used; no network access is required to build.
- Bash, for running the scripts in `tests/`.

## Directory Structure

```text
minimal-layer1-blockchain/
├── src/                Go module "minimal-layer1-blockchain"
│   ├── cmd/sim         CLI: runs a scenario and writes a log
│   ├── encoding        deterministic binary encoding (for everything that is hashed/signed)
│   ├── crypto          keys, hashes, signing/verification with domain separation
│   ├── types           Tx, BlockHeader, Block, Proposal, Vote, CommitCert, message
│   ├── state           key–value store, key ownership, nonces, state hash
│   ├── mempool         pending transactions: deduplication, replay protection, deterministic ordering
│   ├── ledger          finalized chain and its commit certificates
│   ├── consensus       consensus state machine, VoteSet, locking, timeouts, sync
│   ├── network         simulated network: delay/drop/duplication, GST, rate limits, peer blocking, topology
│   ├── node            combines a validator's consensus, mempool, state, and gossip
│   ├── byzantine       malicious behaviors: silent, equivocate, invalid_proposal, spam
│   ├── workload        Alice/Bob/Carol clients, including forged and replayed transactions
│   ├── sim             virtual-time scheduler, RNG, runner
│   ├── simconfig       scenario-file parsing and validation
│   └── logging         one log line per event, with a fixed format
├── tests/              Go module ".../tests" (replace => ../src; no go.work required)
│   ├── unit/           unit tests for each module
│   ├── e2e/            end-to-end tests T1–T8, variants, and multi-seed sweeps
│   ├── testutil/       shared scenario-running and safety-checking helpers
│   ├── run_all.sh      the ONLY entry point for all checks
│   └── check_determinism.sh
├── config/             scenario files (*.json) and topology files (*.txt)
└── logs/               logs generated at runtime (not committed)
```

## Build

```bash
cd src && go build ./... && cd ..
```

## Run a Scenario

```bash
cd src
go run ./cmd/sim -config ../config/t1_normal.json
```

The command prints one line:

```text
scenario=t1_normal agreed=true final_state=c70a067ba8a2429dbb94d608a1820326471083578b4857c360f4b5f368071cc9 end_ms=4816 log=../logs/t1_normal.log
```

- `agreed=true`: all honest nodes have the same block at every height they have finalized. This is **safety**.
- `final_state`: the state hash after the block at `max_height`. This value is identical across repeated runs.
- `end_ms`: the virtual time at which the run stops. The run stops when all honest nodes reach `max_height`, or when `max_time_ms` is reached.
- By default, logs are written to `logs/<name>.log`. Use `-log <file>` to write to a different location.
- Exit codes: `0` means success; `1` means a safety violation or runtime error; `2` means an invalid configuration (the error is printed along with the field name).

## Run Tests

```bash
bash tests/run_all.sh
```

The script can be run from any directory. It takes approximately 35 seconds and prints `ALL CHECKS PASSED` when all checks pass. It consists of four steps:

1. Build both modules and run `go vet` on them.
2. Run unit tests (`tests/unit`).
3. Run end-to-end tests (`tests/e2e`): T1–T8, their variants, and a sweep. The sweep runs **every** file in `config/` with 3 seeds and checks safety, liveness, certificates, and that honest nodes are not blocked.
4. Check CLI determinism: run every file in `config/` twice and compare the two logs byte for byte.

Some other useful commands:

```bash
SWEEP_SEEDS=20 bash tests/run_all.sh                  # longer sweep (approximately 1 minute)
bash tests/check_determinism.sh config/t7_byzantine.json
cd tests && go test ./e2e -run TestT7 -v              # run only one test group
```

Each e2e test writes its log to `logs/`, for example `logs/T7_byzantine.log`, so you can inspect it after the run.

## Scenarios and Tests T1–T8

| Test | Config | What it checks (tests in `tests/e2e`) |
|---|---|---|
| T1 | `t1_normal.json` | Healthy network: all nodes have the same 10-block chain and state hash (`TestT1Normal`) |
| T2 | `t2_dup_reorder.json` | 50% of messages are duplicated, with delays of 10–500 ms: no vote is counted twice, and no false `EQUIVOCATION` is reported (`TestT2DuplicatesAndReordering`) |
| T3 | `t3_invalid_sig.json` | Transactions with invalid signatures, incorrect domains, or attempts to write another user's key: each transaction is `REJECT`ed for the correct reason, and the resulting chain is identical to a run without malicious transactions (`TestT3InvalidTransactions`) |
| T4 | `t4_replay_tx.json` | An identical transaction is submitted again: `REJECT reason=duplicate_tx`; each transaction appears in the chain only once (`TestT4ReplayedTransactions`) |
| T5 | `t5_presync_drop.json` | 80% message loss before GST (15 s): no two different blocks are committed at the same height, and the chain resumes progressing after GST (`TestT5DropsBeforeGST`) |
| T5′ | `t5_lagging_sync.json` | 10% message loss: lagging nodes catch up through block sync with certificates (`TestT5LaggingNodesCatchUpBySync`) |
| T6 | `t6_silent_proposer.json` | Three proposers crash: `TIMEOUT` occurs, the protocol advances to round 1, and the chain continues (`TestT6CrashedProposers`) |
| T7 | `t7_byzantine.json` | f = 3 Byzantine nodes (2, 5, 8) all equivocate: every honest node logs `EQUIVOCATION` against all three and counts only one vote per validator; no conflicting commits occur (`TestT7ByzantineValidators`) |
| T7′ | `t7_byzantine_drops.json` | T7 plus 20% message loss before GST: the chain remains live thanks to the POL carried by re-proposals (`TestT7EquivocationWithDropsStaysLive`) |
| T8 | `t1_normal.json` × 2 | Two runs produce byte-identical logs and the same final state hash (`TestT8Determinism`, also covered by step 4 of `run_all.sh`) |
| — | `t_topology_file.json` | T1 on a 10-node ring, with 4 neighbors per node (`topology_ring10.txt`) (`TestT1OnTopologyFile`) |

Other e2e tests also check:

- strict rate limiting (`TestT1UnderTightRateLimit`);
- blocking a spammer (`TestSpammerIsBlocked`);
- a silent node (`TestT1WithSilentValidator`);
- a node proposing invalid blocks: a tx with a bad signature, a wrong state hash, a wrong parent, each one `REJECT`ed with the right reason (`TestT1WithInvalidProposer`);
- the Byzantine hook making no changes when the behavior is `honest` (`TestHonestHookChangesNothing`).

## Scenario Files (`config/*.json`)

It is recommended that all fields be explicitly specified, as in the sample files. Unknown fields—for example, a typo such as `drop_prop`—are treated as **errors** and are not ignored. Invalid values, such as a `num_validators` value that does not follow 3f+1 or a probability outside [0, 1], are also rejected. The error message identifies the relevant field.

| Field | Meaning |
|---|---|
| `name`, `chain_id`, `seed` | Scenario name, which is also used as the log filename; chain ID used in signature domain separation; seed for all random sources |
| `num_validators` | n = 3f+1, with a minimum of 8. For example, 10 gives f = 3 and a quorum of 7 |
| `max_height`, `max_time_ms` | Stop when all honest nodes reach `max_height`; enforce a hard stop at `max_time_ms` virtual milliseconds |
| `network.gst_ms`, `delta_ms`, `min_delay_ms` | After GST, every message arrives within `[min_delay_ms, delta_ms]` |
| `network.pre_gst.drop_prob`, `dup_prob`, `max_delay_ms` | Before GST: message-drop probability, message-duplication probability, and maximum delay |
| `network.rate_limit.capacity`, `refill_per_ms`, `queue_limit` | Token bucket for each outgoing link. When tokens run out, messages are queued (`DELAY`); when the queue is full, messages are dropped (`DROP reason=rate`). A `capacity` of 0 disables rate limiting |
| `network.peer_block.window_ms`, `max_msgs`, `block_ms` | A peer that sends more than `max_msgs` messages within a window is blocked (`BLOCK`) for `block_ms`. A `max_msgs` value of 0 disables this feature. Honest nodes send at most approximately 60 messages/second per peer, so a threshold of 500 provides ample headroom |
| `network.topology`, `topology_file` | `"full"` (default), or `"file"`: an edge list in the form `a b`; the path is relative to the config file. The graph must remain connected even after faulty nodes are removed |
| `consensus.timeout_*_ms`, `timeout_delta_ms` | The timeout for round r is `base + r × delta`. `timeout_propose_ms` should be greater than approximately 3Δ (header followed by body) |
| `consensus.max_block_txs` | Maximum number of transactions per block |
| `workload.accounts`, `tx_count`, `interval_ms` | Client sends transaction i at `(i+1) × interval_ms`, from account `i mod number_of_accounts`, to a randomly selected node |
| `workload.inject[]` | Malicious transaction `{kind, at_ms, account, repeat}`. `kind` must be one of: `bad_signature`, `wrong_domain`, `foreign_key`, `replay` |
| `byzantine[]` | `{node, behavior, from_ms}`, with at most f entries. `behavior` must be one of: `crash`, `silent`, `equivocate`, `invalid_proposal`, `spam`, or `honest` (used to test the hook) |

## Log Format

Each event occupies one line. Fields have a fixed order, and hashes are printed as 8-character hexadecimal strings:

```text
t=0000000837 node=07 ev=PRECOMMIT     h=1 r=0 block=ffc2ef8f
```

`t` is the virtual time (ms), `node` is the node writing the log, `ev` is the event, and `h` and `r` are the height and round. These are followed by event-specific `key=value` pairs:

| Event | Fields | Meaning |
|---|---|---|
| `SEND` | `type id to delay` | A message is sent; `delay` is the randomly selected delay |
| `DUP` | `type id to delay` | A duplicate generated by the network (before GST) |
| `RECV` | `type id from` | A message arrives. `from=client` indicates a transaction from the workload |
| `DROP` | `type id to/from reason` | A message is dropped. `reason` is one of `pre_gst`, `rate`, `blocked`, `no_link`, or `unknown_peer` |
| `DELAY` | `type id to reason=rate queued` | A message is queued because the token bucket is empty |
| `BLOCK` / `UNBLOCK` | `type id peer msgs window_ms until` / `type id peer dropped` | The peer guard blocks or unblocks a peer; `type`, `id` and `h` are those of the message that crossed the threshold |
| `NEW_ROUND` | `proposer [skip_from]` | A new round begins; `skip_from` is present if the node skips to a higher round after observing f+1 votes from that round |
| `PROPOSE` | `block txs pol` | The proposer announces a header. `pol` ≥ 0 means it is reproposing a block for which a POL already exists; the proposal carries those 2f+1 prevotes so other nodes can check them themselves |
| `HEADER_ACCEPT` / `BODY_ACCEPT` | `block from` / `block txs` | A valid header is accepted (and the body is then requested); executing the body produces the expected state hash |
| `PREVOTE` / `PRECOMMIT` | `block` (`nil` = NIL) | The node's own vote |
| `LOCK` | `block` [`locked_round replaces`] | A lock is acquired after seeing 2f+1 prevotes; when a POL for another block is seen in a round **after** the lock, the lock moves to that block (rules 4, 5) |
| `TIMEOUT` | `step` | The `propose`, `prevote`, or `precommit` timer fires |
| `COMMIT` | `block txs state [sync_from]` | A block is finalized; `sync_from` is present if it was received through block sync (header + certificate first, body after) |
| `EQUIVOCATION` | `validator type first second` | Evidence that a validator signed two different votes; only the `first` vote is counted (rule 7) |
| `REJECT` | `type id/from reason` | A message, transaction, or block is rejected, with a reason (listed below) |
| `CRASH` / `BYZANTINE` | `finalized` / `behavior` | A scenario fault begins |
| `FINAL` | `state mempool` | Per-node summary at the end; `h` is the finalized height |

`REJECT` reasons:

- **Transactions:** `bad_signature`, `wrong_domain`, `unknown_sender`, `key_not_owned_by_sender`, `bad_nonce`, `stale_nonce`, `duplicate_tx`.
- **Headers and blocks:** `wrong_chain`, `wrong_height`, `bad_parent`, `wrong_proposer`, `bad_header_signature`, `bad_round`, `bad_pol_round`, `bad_pol`, `bad_tx_root`, `bad_state_hash`, `too_many_txs`.
- **Votes:** `unknown_validator`, `bad_vote_type`.
- **Certificates and sync:** `cert_no_quorum`, `cert_bad_vote`, `cert_mismatch`, `cert_nil_block`, `sync_cert_mismatch`.

A few useful commands for inspecting logs:

```bash
grep -E 'ev=COMMIT' logs/t1_normal.log | grep 'node=00'   # chain for node 0
grep -c 'ev=EQUIVOCATION' logs/T7_byzantine.log           # number of equivocation proofs
grep 'ev=REJECT' logs/T3_invalid_sig.log                  # invalid transactions and their reasons
grep -E 'ev=(TIMEOUT|NEW_ROUND)' logs/T6_silent_proposer.log
```
'''
