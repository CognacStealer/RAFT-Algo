package main

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"raft-project/raft"
	"strconv"
	"strings"
	"syscall"
)

func CreateCluster(peers int) (*raft.ClusterSimulator, error) {
	if peers < 0 {
		return nil, errors.New("invalid number of peers")
	}
	cluster := raft.CreateNewCluster(nil, uint64(peers))
	if cluster != nil {
		return cluster, nil
	}
	return nil, errors.New("cluster could not be created")
}

func SetData(cluster *raft.ClusterSimulator, key string, val int, serverParam ...int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	commandToServer := raft.Write{Key: key, Val: val}
	serverId := 0
	if len(serverParam) >= 1 {
		serverId = serverParam[0]
	} else {
		var err error
		serverId, _, err = cluster.CheckUniqueLeader()
		if err != nil {
			return err
		}
	}
	if serverId < 0 {
		return errors.New("unable to submit command to any server")
	}
	success := false
	if success, _, _ = cluster.SubmitToServer(serverId, commandToServer); success {
		return nil
	} else {
		return errors.New("command could not be submitted, try different server(leader)")
	}
}

func GetData(cluster *raft.ClusterSimulator, key string, serverParam ...int) (int, error) {
	if cluster == nil {
		return 0, errors.New("raft cluster not created")
	}
	commandToServer := raft.Read{Key: key}
	serverId := 0
	if len(serverParam) >= 1 {
		serverId = serverParam[0]
	} else {
		var err error
		serverId, _, err = cluster.CheckUniqueLeader()
		if err != nil {
			return 0, err
		}
	}
	if serverId < 0 {
		return 0, errors.New("unable to submit command to any server")
	}
	if success, reply, err := cluster.SubmitToServer(serverId, commandToServer); success {
		if err != nil {
			return -1, err
		} else {
			value, _ := reply.(int)
			return value, nil
		}
	} else {
		return 0, errors.New("command could not be submitted, try different server(leader)")
	}
}

func AddServers(cluster *raft.ClusterSimulator, serverIds []int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	commandToServer := raft.AddServers{ServerIds: serverIds}
	var err error
	serverId, _, err := cluster.CheckUniqueLeader()

	if err != nil {
		return err
	}

	if serverId < 0 {
		return errors.New("unable to submit command to any server")
	}

	if success, _, err := cluster.SubmitToServer(serverId, commandToServer); success {
		if err != nil {
			return err
		} else {
			return nil
		}
	} else {
		return errors.New("command could not be submitted, try different server")
	}
}

func RemoveServers(cluster *raft.ClusterSimulator, serverIds []int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	commandToServer := raft.RemoveServers{ServerIds: serverIds}
	var err error
	serverId, _, err := cluster.CheckUniqueLeader()

	if err != nil {
		return err
	}

	if serverId < 0 {
		return errors.New("unable to submit command to any server")
	}

	if success, _, err := cluster.SubmitToServer(serverId, commandToServer); success {
		if err != nil {
			return err
		} else {
			return nil
		}
	} else {
		return errors.New("command could not be submitted, try different server")
	}
}

func DisconnectPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	err := cluster.DisconnectPeer(uint64(peerId))
	return err
}

func ReconnectPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	err := cluster.ReconnectPeer(uint64(peerId))
	return err
}

func PartitionNetwork(cluster *raft.ClusterSimulator, groups [][]uint64) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if len(groups) < 2 {
		return errors.New("at least two partition groups are required")
	}
	return cluster.Partition(groups)
}

func HealNetwork(cluster *raft.ClusterSimulator) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	return cluster.HealNetwork()
}

func CrashPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	err := cluster.CrashPeer(uint64(peerId))
	return err
}

func RestartPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	err := cluster.RestartPeer(uint64(peerId))
	return err
}

func Shutdown(cluster *raft.ClusterSimulator) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	cluster.Shutdown()
	cluster = nil
	return nil
}

func CheckLeader(cluster *raft.ClusterSimulator) (int, int, error) {
	if cluster == nil {
		return -1, -1, errors.New("raft cluster not created")
	}
	return cluster.CheckUniqueLeader()
}

func Stop(cluster *raft.ClusterSimulator) error {
	if cluster == nil {
		return nil
	}
	cluster.Shutdown()
	cluster = nil
	return nil
}

func PrintMenu() {
	fmt.Println("\n\n           	RAFT MENU: [nodes are 0 indexed]")
	fmt.Println("+---------------------------+------------------------------------+")
	fmt.Println("| Sr |  USER COMMANDS       |      ARGUMENTS                     |")
	fmt.Println("+----+----------------------+------------------------------------+")
	fmt.Println("| 1  | create cluster       |      number of nodes               |")
	fmt.Println("| 2  | set data             |      key, value, peerId (optional) |")
	fmt.Println("| 3  | get data             |      key, peerId (optional)        |")
	fmt.Println("| 4  | disconnect peer      |      peerId                        |")
	fmt.Println("| 5  | reconnect peer       |      peerId                        |")
	fmt.Println("| 6  | crash peer           |      peerId                        |")
	fmt.Println("| 7  | restart peer         |      peerId                        |")
	fmt.Println("| 8  | shutdown             |      _                             |")
	fmt.Println("| 9  | check leader         |      _                             |")
	fmt.Println("| 10 | stop execution       |      _                             |")
	fmt.Println("| 11 | add servers          |      [peerIds]                     |")
	fmt.Println("| 12 | remove servers       |      [peerIds]                     |")
	fmt.Println("+----+----------------------+------------------------------------+")
	fmt.Println("")
}

func main() {
	var input string
	var cluster *raft.ClusterSimulator = nil
	var peers int = 0

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		fmt.Println("SIGNAL RECEIVED")
		Stop(cluster)
		os.Exit(0)
	}()

	gob.Register(raft.Write{})
	gob.Register(raft.Read{})
	gob.Register(raft.AddServers{})
	gob.Register(raft.RemoveServers{})

	// START HTTP API Server
	StartHTTPServer("8080", func() *raft.ClusterSimulator { return cluster }, func(peers int) error {
		if cluster != nil {
			cluster.Shutdown()
			cluster = nil
		}
		newCluster, err := CreateCluster(peers)
		if err != nil {
			return err
		}
		cluster = newCluster
		return nil
	})

	fmt.Println("\n\n=============================================================")
	fmt.Println("=    Ensure that you set[DEBUG=0] in [raft/raft.go] file   =")
	fmt.Println("=============================================================")
	PrintMenu()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("WAITING FOR INPUTS..")
		fmt.Println("")

		var readErr error
		input, readErr = reader.ReadString('\n')
		if readErr != nil && len(input) == 0 {
			if errors.Is(readErr, io.EOF) {
				fmt.Println("[CLI] stdin closed; HTTP dashboard remains available.")
				select {}
			}
			fmt.Printf("input error: %v\n", readErr)
			continue
		}
		tokens := strings.Fields(input)
		if len(tokens) == 0 {
			continue
		}
		command, err0 := strconv.Atoi(tokens[0])
		if err0 != nil {
			fmt.Println("Wrong input")
			continue
		}
		switch command {
		case 1:
			if len(tokens) < 2 {
				fmt.Println("number of peers not passed")
				break
			}
			var err error
			peers, err = strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Println("invalid number of peers")
				break
			}
			cluster, err = CreateCluster(peers)
			if err == nil {
				fmt.Printf("CLUSTER OF %d PEERS CREATED !!!\n", peers)
			} else {
				fmt.Printf("err: %v\n", err)
				cluster = nil
			}
		case 2:
			if len(tokens) < 3 {
				fmt.Println("key or value not passed")
				break
			}
			val, err := strconv.Atoi(tokens[2])
			if err != nil {
				fmt.Println("invalid value passed")
				break
			}
			serverId := 0
			if len(tokens) >= 4 {
				serverId, err = strconv.Atoi(tokens[3])
				if err != nil {
					fmt.Printf("invalid server id %d passed\n", serverId)
					break
				}
				err = SetData(cluster, tokens[1], val, serverId)
			} else {
				err = SetData(cluster, tokens[1], val)
			}
			if err == nil {
				fmt.Printf("WRITE TO KEY %s WITH VALUE %d SUCCESSFUL\n", tokens[1], val)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 3:
			if len(tokens) < 2 {
				fmt.Println("key not passed")
				break
			}
			var err error
			var val int
			serverId := 0
			if len(tokens) >= 3 {
				serverId, err = strconv.Atoi(tokens[2])
				if err != nil {
					fmt.Printf("invalid server id %d passed\n", serverId)
					break
				}
				val, err = GetData(cluster, tokens[1], serverId)
			} else {
				val, err = GetData(cluster, tokens[1])
			}
			if err == nil {
				fmt.Printf("READ KEY %s VALUE %d\n", tokens[1], val)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 4:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}
			err = DisconnectPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d DISCONNECTED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 5:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}
			err = ReconnectPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d RECONNECTED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 6:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}
			err = CrashPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d CRASHED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 7:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}
			err = RestartPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d RESTARTED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 8:
			err := Shutdown(cluster)
			if err == nil {
				fmt.Println("ALL SERVERS STOPPED AND RAFT SERVICE STOPPED")
			} else {
				fmt.Printf("%v\n", err)
			}
			cluster = nil
		case 9:
			leaderId, term, err := CheckLeader(cluster)
			if err == nil {
				fmt.Printf("LEADER ID: %d, TERM: %d\n", leaderId, term)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 10:
			err := Stop(cluster)
			if err == nil {
				fmt.Println("STOPPING EXECUTION, NO INPUTS WILL BE TAKEN FURTHER")
				cluster = nil
				return
			} else {
				fmt.Printf("%v\n", err)
			}
		case 11:
			if len(tokens) < 2 {
				fmt.Println("peer ids not passed")
				break
			}
			serverIds := make([]int, len(tokens)-1)
			var val int
			var err error
			for i := 1; i < len(tokens); i++ {
				val, err = strconv.Atoi(tokens[i])
				if err != nil {
					fmt.Println("Invalid server ID")
					break
				}
				serverIds[i-1] = val
			}
			err = AddServers(cluster, serverIds)
			if err == nil {
				fmt.Printf("Added ServerIDs: %v to cluster", serverIds)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 12:
			if len(tokens) < 2 {
				fmt.Println("peer ids not passed")
				break
			}
			serverIds := make([]int, len(tokens)-1)
			var val int
			var err error
			for i := 1; i < len(tokens); i++ {
				val, err = strconv.Atoi(tokens[i])
				if err != nil {
					fmt.Println("Invalid server ID")
					break
				}
				serverIds[i-1] = val
			}
			err = RemoveServers(cluster, serverIds)
			if err == nil {
				fmt.Printf("Removed ServerIDs: %v from cluster", serverIds)
			} else {
				fmt.Printf("%v\n", err)
			}
		default:
			fmt.Println("Invalid Command")
		}
		fmt.Println("\n---------------------------------------------------------")
		PrintMenu()
	}
}
