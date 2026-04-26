package raft

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort"
)

type LogEntrySnapshot struct {
	Index   uint64 `json:"index"`
	Term    uint64 `json:"term"`
	Command string `json:"command"`
}

type CommitSnapshot struct {
	Index   uint64 `json:"index"`
	Term    uint64 `json:"term"`
	Command string `json:"command"`
}

type NodeSnapshot struct {
	ID          uint64             `json:"id"`
	Role        string             `json:"role"`
	Term        uint64             `json:"term"`
	VotedFor    int                `json:"voted_for"`
	Alive       bool               `json:"alive"`
	Connected   bool               `json:"connected"`
	Reachable   []uint64           `json:"reachable"`
	CommitIndex uint64             `json:"commit_index"`
	LastApplied uint64             `json:"last_applied"`
	Peers       []uint64           `json:"peers"`
	Log         []LogEntrySnapshot `json:"log"`
	Commits     []CommitSnapshot   `json:"commits"`
	State       map[string]int     `json:"state"`
}

type ClusterSnapshot struct {
	Nodes    []NodeSnapshot `json:"nodes"`
	LeaderID int            `json:"leader_id"`
	Term     int            `json:"term"`
}

func commandString(command interface{}) string {
	switch v := command.(type) {
	case Write:
		return fmt.Sprintf("write %s=%d", v.Key, v.Val)
	case Read:
		return fmt.Sprintf("read %s", v.Key)
	case AddServers:
		return fmt.Sprintf("add servers %v", v.ServerIds)
	case RemoveServers:
		return fmt.Sprintf("remove servers %v", v.ServerIds)
	default:
		return fmt.Sprintf("%v", command)
	}
}

func decodeState(db *Database) map[string]int {
	state := make(map[string]int)
	for key, value := range db.Snapshot() {
		if key == "currentTerm" || key == "votedFor" || key == "log" {
			continue
		}
		var decoded int
		if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&decoded); err == nil {
			state[key] = decoded
		}
	}
	return state
}

func (rn *RaftNode) Snapshot(alive bool, connected bool, reachable []uint64, commits []CommitEntry) NodeSnapshot {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	peers := make([]uint64, 0, rn.peerList.Size())
	for peer := range rn.peerList.peerSet {
		peers = append(peers, peer)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i] < peers[j] })
	sort.Slice(reachable, func(i, j int) bool { return reachable[i] < reachable[j] })

	logEntries := make([]LogEntrySnapshot, 0, len(rn.log))
	for i, entry := range rn.log {
		logEntries = append(logEntries, LogEntrySnapshot{
			Index:   uint64(i) + 1,
			Term:    entry.Term,
			Command: commandString(entry.Command),
		})
	}

	commitEntries := make([]CommitSnapshot, 0, len(commits))
	for _, commit := range commits {
		commitEntries = append(commitEntries, CommitSnapshot{
			Index:   commit.Index,
			Term:    commit.Term,
			Command: commandString(commit.Command),
		})
	}

	return NodeSnapshot{
		ID:          rn.id,
		Role:        rn.state.String(),
		Term:        rn.currentTerm,
		VotedFor:    rn.votedFor,
		Alive:       alive,
		Connected:   connected,
		Reachable:   reachable,
		CommitIndex: rn.commitIndex,
		LastApplied: rn.lastApplied,
		Peers:       peers,
		Log:         logEntries,
		Commits:     commitEntries,
		State:       decodeState(rn.db),
	}
}

func (nc *ClusterSimulator) Snapshot() ClusterSnapshot {
	nc.mu.Lock()
	type nodeData struct {
		id        uint64
		server    *Server
		alive     bool
		connected bool
		commits   []CommitEntry
		reachable []uint64
	}

	nodesData := make([]nodeData, 0, nc.activeServers.Size())
	for id := range nc.activeServers.peerSet {
		commits := append([]CommitEntry(nil), nc.commits[id]...)
		reachable := nc.ReachablePeersLocked(id)
		nodesData = append(nodesData, nodeData{
			id:        id,
			server:    nc.raftCluster[id],
			alive:     nc.isAlive[id],
			connected: nc.isConnected[id],
			commits:   commits,
			reachable: reachable,
		})
	}
	nc.mu.Unlock()

	sort.Slice(nodesData, func(i, j int) bool { return nodesData[i].id < nodesData[j].id })

	snapshot := ClusterSnapshot{
		Nodes:    make([]NodeSnapshot, 0, len(nodesData)),
		LeaderID: -1,
		Term:     -1,
	}
	for _, data := range nodesData {
		if data.server == nil || data.server.rn == nil {
			continue
		}
		nodeSnapshot := data.server.rn.Snapshot(data.alive, data.connected, data.reachable, data.commits)
		if nodeSnapshot.Role == Leader.String() && nodeSnapshot.Connected && nc.HasQuorum(nodeSnapshot.ID) {
			snapshot.LeaderID = int(nodeSnapshot.ID)
			snapshot.Term = int(nodeSnapshot.Term)
		}
		snapshot.Nodes = append(snapshot.Nodes, nodeSnapshot)
	}

	return snapshot
}
