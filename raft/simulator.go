package raft

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	seed := time.Now().UnixNano()
	fmt.Println("Seed: ", seed)

	rand.Seed(seed)
}

type ClusterSimulator struct {
	mu sync.Mutex

	raftCluster map[uint64]*Server
	dbCluster   map[uint64]*Database

	commitChans map[uint64]chan CommitEntry

	commits map[uint64][]CommitEntry

	isConnected map[uint64]bool
	links       map[uint64]map[uint64]bool

	isAlive map[uint64]bool

	activeServers Set
	n             uint64
	t             *testing.T
}

type CommitFunctionType int

const (
	TestCommitFunction CommitFunctionType = iota
	TestNoCommitFunction
)

type Write struct {
	Key string
	Val int
}

type Read struct {
	Key string
}

type AddServers struct {
	ServerIds []int
}

type RemoveServers struct {
	ServerIds []int
}

func CreateNewCluster(t *testing.T, n uint64) *ClusterSimulator {
	serverList := make(map[uint64]*Server)
	isConnected := make(map[uint64]bool)
	links := make(map[uint64]map[uint64]bool)
	isAlive := make(map[uint64]bool)
	commitChans := make(map[uint64]chan CommitEntry)
	commits := make(map[uint64][]CommitEntry)
	ready := make(chan interface{})
	storage := make(map[uint64]*Database)
	activeServers := makeSet()

	for i := uint64(0); i < n; i++ {
		peerList := makeSet()

		for j := uint64(0); j < n; j++ {
			if i == j {
				continue
			} else {
				peerList.Add(j)
			}
		}

		storage[i] = NewDatabase()
		commitChans[i] = make(chan CommitEntry)
		serverList[i] = CreateServer(i, peerList, storage[i], ready, commitChans[i])
		links[i] = make(map[uint64]bool)

		serverList[i].Serve()
		isAlive[i] = true
	}

	for i := uint64(0); i < n; i++ {
		for j := uint64(0); j < n; j++ {
			if i == j {
				continue
			}
			serverList[i].ConnectToPeer(j, serverList[j].GetListenerAddr())
			links[i][j] = true
		}
		isConnected[i] = true
	}

	for i := uint64(0); i < n; i++ {
		activeServers.Add(i)
	}
	close(ready)

	newCluster := &ClusterSimulator{
		raftCluster:   serverList,
		dbCluster:     storage,
		commitChans:   commitChans,
		commits:       commits,
		isConnected:   isConnected,
		links:         links,
		isAlive:       isAlive,
		n:             n,
		activeServers: activeServers,
		t:             t,
	}

	for i := uint64(0); i < n; i++ {
		go newCluster.collectCommits(i)
	}

	return newCluster
}

func (nc *ClusterSimulator) Shutdown() {
	for i := range nc.activeServers.peerSet {
		nc.raftCluster[i].DisconnectAll()
		nc.isConnected[i] = false
		for j := range nc.activeServers.peerSet {
			nc.setLinkLocked(i, j, false)
		}
	}

	for i := range nc.activeServers.peerSet {
		if nc.isAlive[i] {
			nc.isAlive[i] = false
			nc.raftCluster[i].Stop()
		}
	}

	for i := range nc.activeServers.peerSet {
		close(nc.commitChans[i])
	}
}

func (nc *ClusterSimulator) collectCommits(i uint64) error {
	for commit := range nc.commitChans[i] {
		nc.mu.Lock()
		logtest(i, "collectCommits (%d) got %+v", i, commit)
		switch v := commit.Command.(type) {
		case Read:
			break
		case Write:
			var buf bytes.Buffer
			enc := gob.NewEncoder(&buf)

			if err := enc.Encode(v.Val); err != nil {
				nc.mu.Unlock()
				return err
			}
			nc.dbCluster[i].Set(v.Key, buf.Bytes())
		case RemoveServers:
			serverIds := v.ServerIds
			for j := uint64(0); j < uint64(len(serverIds)); j++ {
				targetId := uint64(serverIds[j])
				if nc.activeServers.Exists(targetId) {
					nc.DisconnectPeer(targetId)
					nc.isAlive[targetId] = false
					nc.raftCluster[targetId].Stop()
					nc.commits[targetId] = nc.commits[targetId][:0]

					// FIX: Close the channel to free the goroutine and stop the leak.
					close(nc.commitChans[targetId])

					delete(nc.raftCluster, targetId)
					delete(nc.dbCluster, targetId)
					delete(nc.commitChans, targetId)
					delete(nc.commits, targetId)
					delete(nc.isAlive, targetId)
					delete(nc.isConnected, targetId)
					delete(nc.links, targetId)
					for peer := range nc.links {
						delete(nc.links[peer], targetId)
					}

					nc.activeServers.Remove(targetId)
				}
			}
		default:
			break
		}
		nc.commits[i] = append(nc.commits[i], commit)
		nc.mu.Unlock()
	}
	return nil
}

func (nc *ClusterSimulator) DisconnectPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	logtest(id, "Disconnect %d", id)

	nc.raftCluster[id].DisconnectAll()
	for i := range nc.activeServers.peerSet {
		if i == id {
			continue
		} else {
			nc.raftCluster[i].DisconnectPeer(id)
			nc.setLinkLocked(id, i, false)
		}
	}
	nc.isConnected[id] = false
	return nil
}

func (nc *ClusterSimulator) ReconnectPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	logtest(id, "Reconnect %d", id)

	for i := range nc.activeServers.peerSet {
		if i != id && nc.isAlive[i] {
			err := nc.raftCluster[id].ConnectToPeer(i, nc.raftCluster[i].GetListenerAddr())
			if err != nil {
				if nc.t != nil {
					nc.t.Fatal(err)
				} else {
					return err
				}
			}
			err = nc.raftCluster[i].ConnectToPeer(id, nc.raftCluster[id].GetListenerAddr())
			if err != nil {
				if nc.t != nil {
					nc.t.Fatal(err)
				} else {
					return err
				}
			}
			nc.setLinkLocked(id, i, true)
		}
	}

	nc.isConnected[id] = true
	return nil
}

func (nc *ClusterSimulator) setLinkLocked(a uint64, b uint64, connected bool) {
	if a == b {
		return
	}
	if nc.links == nil {
		nc.links = make(map[uint64]map[uint64]bool)
	}
	if nc.links[a] == nil {
		nc.links[a] = make(map[uint64]bool)
	}
	if nc.links[b] == nil {
		nc.links[b] = make(map[uint64]bool)
	}
	nc.links[a][b] = connected
	nc.links[b][a] = connected
}

func (nc *ClusterSimulator) ReachablePeersLocked(id uint64) []uint64 {
	reachable := make([]uint64, 0)
	for peer, connected := range nc.links[id] {
		if connected && nc.activeServers.Exists(peer) && nc.isAlive[peer] {
			reachable = append(reachable, peer)
		}
	}
	return reachable
}

func (nc *ClusterSimulator) HealNetwork() error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	for id := range nc.activeServers.peerSet {
		if !nc.isAlive[id] {
			nc.isConnected[id] = false
			continue
		}
		nc.raftCluster[id].DisconnectAll()
	}

	for id := range nc.activeServers.peerSet {
		if !nc.isAlive[id] {
			continue
		}
		for peer := range nc.activeServers.peerSet {
			if id == peer || !nc.isAlive[peer] {
				continue
			}
			if err := nc.raftCluster[id].ConnectToPeer(peer, nc.raftCluster[peer].GetListenerAddr()); err != nil {
				return err
			}
			nc.setLinkLocked(id, peer, true)
		}
		nc.isConnected[id] = true
	}

	return nil
}

func (nc *ClusterSimulator) Partition(groups [][]uint64) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	seen := make(map[uint64]bool)
	for _, group := range groups {
		for _, id := range group {
			if !nc.activeServers.Exists(id) {
				return fmt.Errorf("invalid server id %d passed", id)
			}
			if seen[id] {
				return fmt.Errorf("server id %d appears in more than one partition", id)
			}
			seen[id] = true
		}
	}

	for id := range nc.activeServers.peerSet {
		if nc.isAlive[id] {
			nc.raftCluster[id].DisconnectAll()
		}
		nc.isConnected[id] = false
		for peer := range nc.activeServers.peerSet {
			nc.setLinkLocked(id, peer, false)
		}
	}

	for _, group := range groups {
		for _, id := range group {
			if !nc.isAlive[id] {
				continue
			}
			nc.isConnected[id] = true
			for _, peer := range group {
				if id == peer || !nc.isAlive[peer] {
					continue
				}
				if err := nc.raftCluster[id].ConnectToPeer(peer, nc.raftCluster[peer].GetListenerAddr()); err != nil {
					return err
				}
				nc.setLinkLocked(id, peer, true)
			}
		}
	}

	return nil
}

func (nc *ClusterSimulator) CrashPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	logtest(id, "Crash %d", id)

	nc.DisconnectPeer(id)

	nc.isAlive[id] = false
	nc.raftCluster[id].Stop()

	nc.mu.Lock()
	nc.commits[id] = nc.commits[id][:0]
	nc.mu.Unlock()
	return nil
}

func (nc *ClusterSimulator) RestartPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	if nc.isAlive[id] {
		if nc.t != nil {
			log.Fatalf("Id %d alive in restart peer", id)
		} else {
			return fmt.Errorf("id %d alive in restart peer", id)
		}
	}
	logtest(id, "Restart ", id, id)

	peerList := makeSet()
	for i := range nc.activeServers.peerSet {
		if id == i {
			continue
		} else {
			peerList.Add(i)
		}
	}

	ready := make(chan interface{})

	nc.raftCluster[id] = CreateServer(id, peerList, nc.dbCluster[id], ready, nc.commitChans[id])
	nc.raftCluster[id].Serve()
	nc.ReconnectPeer(id)

	close(ready)
	nc.isAlive[id] = true

	time.Sleep(time.Duration(20) * time.Millisecond)
	return nil
}

func (nc *ClusterSimulator) CheckUniqueLeader() (int, int, error) {
	for r := 0; r < 8; r++ {
		leaderId := -1
		leaderTerm := -1

		for i := range nc.activeServers.peerSet {
			if nc.isConnected[i] {
				_, term, isLeader := nc.raftCluster[i].rn.Report()
				if isLeader && nc.HasQuorum(i) {
					if leaderId < 0 {
						leaderId = int(i)
						leaderTerm = term
					} else {
						if nc.t != nil {
							nc.t.Fatalf("2 ids: %d, %d think they are leaders", leaderId, i)
						} else {
							return -1, -1, fmt.Errorf("2 ids: %d, %d think they are leaders", leaderId, i)
						}
					}
				}
			}
		}
		if leaderId >= 0 {
			return leaderId, leaderTerm, nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	if nc.t != nil {
		nc.t.Fatalf("no leader found")
	}
	return -1, -1, fmt.Errorf("no leader found")
}

func (nc *ClusterSimulator) HasQuorum(id uint64) bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if !nc.activeServers.Exists(id) || !nc.isAlive[id] || !nc.isConnected[id] {
		return false
	}
	votes := 1
	for peer, connected := range nc.links[id] {
		if connected && nc.activeServers.Exists(peer) && nc.isAlive[peer] {
			votes++
		}
	}
	return votes*2 > nc.activeServers.Size()
}

func (nc *ClusterSimulator) CheckNoLeader() error {
	for i := range nc.activeServers.peerSet {
		if nc.isConnected[i] {
			if _, _, isLeader := nc.raftCluster[i].rn.Report(); isLeader {
				if nc.t != nil {
					nc.t.Fatalf("%d is Leader, expected no leader", i)
				} else {
					return fmt.Errorf("%d is Leader, expected no leader", i)
				}
			}
		}
	}
	return nil
}

func (nc *ClusterSimulator) CheckCommitted(cmd int, choice CommitFunctionType) (num int, index int, err error) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	err = nil

	commitsLen := -1
	for i := range nc.activeServers.peerSet {
		if nc.isConnected[i] {
			if commitsLen >= 0 {
				if len(nc.commits[i]) != commitsLen {
					if nc.t != nil {
						nc.t.Fatalf("commits[%d] = %d, commitsLen = %d", i, nc.commits[i], commitsLen)
					} else {
						err = fmt.Errorf("commits[%d] = %d, commitsLen = %d", i, nc.commits[i], commitsLen)
						return -1, -1, err
					}
				}
			} else {
				commitsLen = len(nc.commits[i])
			}
		}
	}

	for c := 0; c < commitsLen; c++ {
		cmdAtC := -1
		for i := range nc.activeServers.peerSet {
			if nc.isConnected[i] {
				cmdOfN := nc.commits[i][c].Command.(int)
				if cmdAtC >= 0 {
					if cmdOfN != cmdAtC {
						if nc.t != nil {
							nc.t.Errorf("got %d, want %d at nc.commits[%d][%d]", cmdOfN, cmdAtC, i, c)
						} else {
							err = fmt.Errorf("got %d, want %d at nc.commits[%d][%d]", cmdOfN, cmdAtC, i, c)
						}
					}
				} else {
					cmdAtC = cmdOfN
				}
			}
		}
		if cmdAtC == cmd {
			index := -1
			num := 0
			for i := range nc.activeServers.peerSet {
				if nc.isConnected[i] {
					if index >= 0 && int(nc.commits[i][c].Index) != index {
						if nc.t != nil {
							nc.t.Errorf("got Index=%d, want %d at h.commits[%d][%d]", nc.commits[i][c].Index, index, i, c)
						} else {
							err = fmt.Errorf("got Index=%d, want %d at h.commits[%d][%d]", nc.commits[i][c].Index, index, i, c)
						}
					} else {
						index = int(nc.commits[i][c].Index)
					}
					num++
				}
			}
			return num, index, err
		}
	}

	if choice == TestCommitFunction {
		if nc.t != nil {
			nc.t.Errorf("cmd = %d not found in commits", cmd)
		} else {
			err = fmt.Errorf("cmd = %d not found in commits", cmd)
		}
		return 0, -1, err
	} else {
		return 0, -1, err
	}
}

func (nc *ClusterSimulator) SubmitToServer(serverId int, cmd interface{}) (bool, interface{}, error) {
	if !nc.activeServers.Exists(uint64(serverId)) {
		return false, nil, fmt.Errorf("invalid server id passed")
	}
	switch v := cmd.(type) {
	case AddServers:
		nc.mu.Lock()
		serverIds := v.ServerIds
		for i := 0; i < len(serverIds); i++ {
			nc.activeServers.Add(uint64(serverIds[i]))
		}
		ready := make(chan interface{})

		for i := uint64(0); i < uint64(len(serverIds)); i++ {
			peerList := makeSet()

			for j := range nc.activeServers.peerSet {
				if uint64(serverIds[i]) == j {
					continue
				} else {
					peerList.Add(j)
				}
			}

			nc.dbCluster[uint64(serverIds[i])] = NewDatabase()
			nc.commitChans[uint64(serverIds[i])] = make(chan CommitEntry)
			nc.raftCluster[uint64(serverIds[i])] = CreateServer(uint64(serverIds[i]), peerList, nc.dbCluster[uint64(serverIds[i])], ready, nc.commitChans[uint64(serverIds[i])])
			nc.links[uint64(serverIds[i])] = make(map[uint64]bool)

			nc.raftCluster[uint64(serverIds[i])].Serve()
			nc.isAlive[uint64(serverIds[i])] = true
		}

		for i := uint64(0); i < uint64(len(serverIds)); i++ {
			for j := range nc.activeServers.peerSet {
				if uint64(serverIds[i]) == j {
					continue
				}
				nc.raftCluster[uint64(serverIds[i])].ConnectToPeer(j, nc.raftCluster[j].GetListenerAddr())
				nc.raftCluster[j].ConnectToPeer(uint64(serverIds[i]), nc.raftCluster[uint64(serverIds[i])].GetListenerAddr())
				nc.setLinkLocked(uint64(serverIds[i]), j, true)
			}
			nc.isConnected[uint64(serverIds[i])] = true
		}

		for i := uint64(0); i < uint64(len(serverIds)); i++ {
			go nc.collectCommits(uint64(serverIds[i]))
		}

		close(ready)

		nc.mu.Unlock()
		return nc.raftCluster[uint64(serverId)].rn.Submit(cmd)
	case RemoveServers:
		return nc.raftCluster[uint64(serverId)].rn.Submit(cmd)
	default:
		return nc.raftCluster[uint64(serverId)].rn.Submit(cmd)
	}
}

func logtest(id uint64, logstr string, a ...interface{}) {
	if DEBUG > 0 {
		logstr = "[" + strconv.Itoa(int(id)) + "] " + "[TEST]" + logstr
		log.Printf(logstr, a...)
	}
}
