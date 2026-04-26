```markdown
# Raft Consensus Simulator & Learning Dashboard

A Go-based Raft consensus simulator with an interactive CLI, HTTP API, browser dashboard, replicated key-value state machine, crash/restart simulation, and network partition controls.

This project is meant for learning how distributed systems behave under failure.

## What This Project Teaches

- Leader election
- Log replication
- Majority commit
- Replicated state machines
- Crash-stop failures
- Node restart and catch-up
- Network partitions
- Split-brain prevention
- CAP theorem tradeoffs
- Why minority partitions cannot safely commit writes

## Project Structure

```text
.
├── main.go              # CLI entry point and HTTP server startup
├── http_api.go          # HTTP API and browser dashboard
├── go.mod               # Go module definition
├── go.sum               # Go dependency lock file
├── RAFT Algo.pdf        # Reference/notes PDF
├── raft/
│   ├── raft.go          # Core Raft logic
│   ├── server.go        # RPC server wrapper for each Raft node
│   ├── simulator.go     # Cluster simulator and fault controls
│   ├── storage.go       # In-memory key-value storage
│   ├── snapshot.go      # Dashboard inspection snapshots
│   └── config.go        # Small set/config helper
└── viz/
    └── main.go          # Terminal log visualizer
```

## Requirements

Install Go.

Check your Go version:

```powershell
go version
```

This project uses Go modules, so dependencies are handled automatically.

## How To Run

From the project folder:

```powershell
cd "C:\Users\admin\Desktop\Work\Distributed Project"
go run .
```

Then open the dashboard:

```text
http://127.0.0.1:8080
```

You can also use the CLI menu in the terminal while the dashboard is running.

## First Demo: Normal Raft Replication

1. Open the dashboard.
2. Set peers to `3`.
3. Click **Create Cluster**.
4. Wait until one node becomes **Leader**.
5. Set:

```text
key: x
value: 10
```

6. Watch each node.

You should see:

```text
Replicated Log:
#1 T1 write x=10

State Machine:
x 10
```

This means the write was replicated, committed, and applied.

## Second Demo: Leader Crash

1. Create a `3` or `5` node cluster.
2. Wait for a leader.
3. Click **Crash** on the leader.
4. Wait a moment.

Expected behavior:

- The old leader becomes unavailable.
- Another node becomes leader.
- The cluster continues if a majority is still alive.

Then write another value:

```text
key: y
value: 20
```

The healthy majority should still replicate and commit it.

## Third Demo: Restart And Catch-Up

1. Crash a node.
2. Write new values while it is crashed.
3. Click **Restart** on that node.
4. Watch its log and state machine.

Expected behavior:

- The restarted node should receive missing log entries.
- Its state machine should eventually match the cluster.

## Fourth Demo: Network Partition

Use a `5` node cluster for this.

1. Create a cluster with `5` peers.
2. Wait for a leader.
3. Click **Majority Split**.

This creates a split like:

```text
0,1,2 | 3,4
```

The majority side has 3 nodes.

Expected behavior:

- Majority side can elect/keep a valid leader.
- Minority side cannot safely commit writes.
- The dashboard shows each node’s reachable peers.

Now write:

```text
key: partition_test
value: 100
```

The majority side should commit it.

Then click **Heal Network**.

Expected behavior:

- All nodes reconnect.
- Minority nodes catch up with the committed log.

## Fifth Demo: Isolate Leader

1. Create a `5` node cluster.
2. Wait for a leader.
3. Click **Isolate Leader**.

This separates the current leader from the rest of the cluster.

Expected behavior:

- The isolated old leader loses quorum.
- The remaining majority elects a new valid leader.
- The dashboard reports the quorum leader, not the isolated stale leader.

This demonstrates how Raft avoids split brain.

## Important Concepts

### Leader Election

Nodes begin as followers.

If a follower stops hearing from a leader, it becomes a candidate and asks for votes.

```text
Follower -> Candidate -> Leader
```

A candidate must receive a majority of votes to become leader.

### Log Replication

Clients submit writes to the leader.

The leader appends the command to its log, then sends it to followers with `AppendEntries`.

A log entry becomes committed only after a majority has accepted it.

### Replicated State Machine

The state machine is the key-value database.

Example command:

```text
write x=10
```

Once committed, every healthy node applies the command to its local database.

That is how all nodes eventually agree on the same state.

### Quorum / Majority

Raft requires a majority to make progress.

For example:

```text
3 nodes -> majority is 2
5 nodes -> majority is 3
7 nodes -> majority is 4
```

This prevents two different groups from both safely committing conflicting writes.

### Network Partition

A network partition happens when nodes are alive but cannot communicate across groups.

Example:

```text
0,1,2 | 3,4
```

Raft allows the majority side to continue and prevents the minority side from committing.

### Split Brain

Split brain is when two parts of a distributed system both believe they are the real leader.

Raft prevents this using majority voting. Two different leaders cannot both have a majority in the same term.

### CAP Theorem

During a network partition, a system must choose between consistency and availability.

Raft chooses:

```text
Consistency + Partition tolerance
```

That means the minority side may become unavailable for writes, but the system avoids conflicting data.

### Crash-Stop vs Byzantine Failures

This project handles crash-stop failures:

```text
node crashes
node restarts
node disconnects
network partitions
```

It does not handle Byzantine failures, where nodes lie or behave maliciously.

Raft is not designed for Byzantine failures.

## Dashboard Features

The dashboard shows:

- Current quorum leader
- Current term
- Number of nodes
- Majority size
- Node role
- Node term
- Voted-for server
- Alive/connected status
- Reachable peers
- Replicated log
- Committed entries
- State machine values

Dashboard actions:

- Create cluster
- Set key-value data
- Crash node
- Restart node
- Disconnect node
- Reconnect node
- Apply custom partition
- Majority split
- Isolate leader
- Heal network

## HTTP API

### Get Cluster State

```http
GET /api/cluster
```

Returns all node snapshots.

### Create Cluster

```http
POST /api/cluster
Content-Type: application/json

{
  "peers": 5
}
```

### Write Data

```http
POST /set
Content-Type: application/json

{
  "key": "x",
  "val": 10
}
```

### Read Data

```http
GET /get?key=x
```

### Check Leader

```http
GET /leader
```

### Crash A Peer

```http
POST /api/peer/0/crash
```

### Restart A Peer

```http
POST /api/peer/0/restart
```

### Disconnect A Peer

```http
POST /api/peer/0/disconnect
```

### Reconnect A Peer

```http
POST /api/peer/0/reconnect
```

### Apply Network Partition

```http
POST /api/network/partition
Content-Type: application/json

{
  "groups": [[0, 1, 2], [3, 4]]
}
```

### Heal Network

```http
POST /api/network/heal
```

## CLI Commands

When running `go run .`, the terminal menu supports:

```text
1  create cluster
2  set data
3  get data
4  disconnect peer
5  reconnect peer
6  crash peer
7  restart peer
8  shutdown
9  check leader
10 stop execution
11 add servers
12 remove servers
```

Example CLI session:

```text
1 5
2 x 10
9
6 0
7 0
3 x
```

## How To Verify The Project

Run:

```powershell
go test ./...
```

Expected result:

```text
?    raft-project       [no test files]
?    raft-project/raft  [no test files]
?    raft-project/viz   [no test files]
```

This means the project compiles successfully.

Note: there are currently no automated test files.

## Suggested Learning Path

Follow this order:

1. Create a 3-node cluster.
2. Observe leader election.
3. Write one key-value pair.
4. Watch log replication.
5. Crash the leader.
6. Observe re-election.
7. Restart the crashed node.
8. Watch catch-up.
9. Create a 5-node cluster.
10. Apply majority partition.
11. Write during partition.
12. Heal network.
13. Isolate leader.
14. Observe split-brain prevention.

## Current Limitations

This is a learning/demo project, not a production Raft system.

Current limitations:

- Storage is in-memory, not durable disk storage.
- Reads are served directly by the leader.
- Snapshotting/log compaction is not implemented.
- Membership changes are simplified.
- There is no automated test suite yet.
- Byzantine failures are not supported.

## Good Next Improvements

Recommended next features:

1. Automated Raft test suite
2. Event timeline in dashboard
3. Persistent disk-backed Raft log
4. Snapshot/log compaction
5. Safer linearizable reads
6. More detailed RPC message visualization
7. Exportable experiment logs

## One-Sentence Summary

This project is a visual Raft simulator that shows how a distributed cluster keeps one consistent replicated state machine alive even when leaders crash, nodes disconnect, and the network splits.
```
