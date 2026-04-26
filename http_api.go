package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"raft-project/raft"
	"strconv"
	"strings"
)

// HTTPAPI wraps the cluster state to serve requests
type HTTPAPI struct {
	getCluster func() *raft.ClusterSimulator
	create     func(peers int) error
}

type SetRequest struct {
	Key string `json:"key"`
	Val int    `json:"val"`
}

type CreateClusterRequest struct {
	Peers int `json:"peers"`
}

type PartitionRequest struct {
	Groups [][]uint64 `json:"groups"`
}

// StartHTTPServer launches the API in the background
func StartHTTPServer(port string, clusterProvider func() *raft.ClusterSimulator, createCluster func(peers int) error) {
	api := &HTTPAPI{getCluster: clusterProvider, create: createCluster}

	http.HandleFunc("/", api.handleDashboard)
	http.HandleFunc("/get", api.handleGet)
	http.HandleFunc("/set", api.handleSet)
	http.HandleFunc("/leader", api.handleLeader)
	http.HandleFunc("/api/cluster", api.handleCluster)
	http.HandleFunc("/api/network/heal", api.handleHealNetwork)
	http.HandleFunc("/api/network/partition", api.handlePartitionNetwork)
	http.HandleFunc("/api/peer/", api.handlePeerAction)

	fmt.Printf("[API] HTTP API Server running on http://localhost:%s\n", port)
	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			fmt.Printf("HTTP Server Error: %v\n", err)
		}
	}()
}

func (api *HTTPAPI) handleHealNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cluster := api.getCluster()
	if cluster == nil {
		http.Error(w, "Cluster not initialized", http.StatusServiceUnavailable)
		return
	}
	if err := HealNetwork(cluster); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "healed"})
}

func (api *HTTPAPI) handlePartitionNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cluster := api.getCluster()
	if cluster == nil {
		http.Error(w, "Cluster not initialized", http.StatusServiceUnavailable)
		return
	}

	var req PartitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := PartitionNetwork(cluster, req.Groups); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "partitioned", "groups": req.Groups})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (api *HTTPAPI) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

func (api *HTTPAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	cluster := api.getCluster()
	if cluster == nil {
		http.Error(w, "Cluster not initialized", http.StatusServiceUnavailable)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Missing 'key' query parameter", http.StatusBadRequest)
		return
	}

	val, err := GetData(cluster, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"key": key, "value": val})
}

func (api *HTTPAPI) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cluster := api.getCluster()
	if cluster == nil {
		http.Error(w, "Cluster not initialized", http.StatusServiceUnavailable)
		return
	}

	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	err := SetData(cluster, req.Key, req.Val)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"status": "success", "key": req.Key, "value": req.Val})
}

func (api *HTTPAPI) handleLeader(w http.ResponseWriter, r *http.Request) {
	cluster := api.getCluster()
	if cluster == nil {
		http.Error(w, "Cluster not initialized", http.StatusServiceUnavailable)
		return
	}

	leaderId, term, err := CheckLeader(cluster)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"leader_id": leaderId, "term": term})
}

func (api *HTTPAPI) handleCluster(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cluster := api.getCluster()
		if cluster == nil {
			writeJSON(w, map[string]interface{}{"initialized": false, "nodes": []interface{}{}})
			return
		}
		snapshot := cluster.Snapshot()
		writeJSON(w, map[string]interface{}{
			"initialized": true,
			"leader_id":   snapshot.LeaderID,
			"term":        snapshot.Term,
			"nodes":       snapshot.Nodes,
		})
	case http.MethodPost:
		var req CreateClusterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.Peers <= 0 {
			http.Error(w, "peers must be greater than zero", http.StatusBadRequest)
			return
		}
		if err := api.create(req.Peers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{"status": "created", "peers": req.Peers})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *HTTPAPI) handlePeerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cluster := api.getCluster()
	if cluster == nil {
		http.Error(w, "Cluster not initialized", http.StatusServiceUnavailable)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/peer/"), "/")
	if len(parts) != 2 {
		http.Error(w, "Expected /api/peer/{id}/{action}", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id < 0 {
		http.Error(w, "Invalid peer id", http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "disconnect":
		err = DisconnectPeer(cluster, id)
	case "reconnect":
		err = ReconnectPeer(cluster, id)
	case "crash":
		err = CrashPeer(cluster, id)
	case "restart":
		err = RestartPeer(cluster, id)
	default:
		http.Error(w, "Unknown peer action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "ok", "peer": id, "action": parts[1]})
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Raft Cluster Dashboard</title>
<style>
:root {
  color-scheme: light;
  --bg: #eef3f8;
  --ink: #17212f;
  --muted: #667085;
  --line: #d3dce8;
  --panel: #ffffff;
  --panel-soft: #f8fafc;
  --accent: #215a8e;
  --leader: #0f9f6e;
  --candidate: #bd7b00;
  --follower: #316fd6;
  --dead: #9b1c31;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  background: var(--bg);
  color: var(--ink);
}
header {
  padding: 18px 24px;
  border-bottom: 1px solid var(--line);
  background: #ffffff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  position: sticky;
  top: 0;
  z-index: 2;
}
h1 { font-size: 22px; margin: 0; letter-spacing: 0; }
main { padding: 20px 24px 28px; }
h2 { font-size: 18px; margin: 0; }
h3 { font-size: 13px; margin: 0 0 10px; color: var(--muted); text-transform: uppercase; }
button, input {
  height: 36px;
  border: 1px solid var(--line);
  background: #ffffff;
  color: var(--ink);
  border-radius: 6px;
  padding: 0 12px;
  font: inherit;
}
button { cursor: pointer; }
button.primary { background: var(--accent); color: #ffffff; border-color: var(--accent); }
button.danger { color: var(--dead); }
button:disabled { opacity: .55; cursor: default; }
.toolbar, .forms, .button-row {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}
.shell {
  display: grid;
  grid-template-columns: 360px minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.control-panel {
  display: grid;
  gap: 12px;
}
.panel {
  margin-bottom: 18px;
  padding: 14px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
}
.panel p {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
  margin: 8px 0 0;
}
.content { min-width: 0; }
.summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 18px;
}
.metric {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 14px;
}
.metric.good { border-left: 4px solid var(--leader); }
.metric.warn { border-left: 4px solid var(--candidate); }
.metric span { color: var(--muted); font-size: 13px; display: block; }
.metric strong { font-size: 24px; }
.nodes {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(310px, 1fr));
  gap: 14px;
}
.node {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 14px;
  box-shadow: 0 10px 24px rgba(31, 48, 70, .05);
}
.node-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 12px;
}
.role {
  display: inline-flex;
  align-items: center;
  min-width: 88px;
  justify-content: center;
  height: 26px;
  border-radius: 999px;
  color: #ffffff;
  font-size: 12px;
  font-weight: 700;
}
.Leader { background: var(--leader); }
.Candidate { background: var(--candidate); }
.Follower { background: var(--follower); }
.Dead { background: var(--dead); }
.facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  color: var(--muted);
  font-size: 13px;
}
.section-title {
  margin: 14px 0 8px;
  font-size: 13px;
  color: var(--muted);
  text-transform: uppercase;
}
.log, .state {
  border: 1px solid var(--line);
  border-radius: 6px;
  overflow: hidden;
  background: var(--panel-soft);
  max-height: 180px;
  overflow-y: auto;
}
.row {
  display: grid;
  grid-template-columns: 44px 54px 1fr;
  gap: 8px;
  padding: 8px 10px;
  border-top: 1px solid var(--line);
  font-size: 13px;
}
.row:first-child { border-top: 0; }
.state .row { grid-template-columns: 1fr 80px; }
.empty { padding: 10px; color: var(--muted); font-size: 13px; }
.actions {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
#message { color: var(--muted); font-size: 13px; }
@media (max-width: 760px) {
  header { align-items: flex-start; flex-direction: column; }
  main { padding: 14px; }
  .shell { grid-template-columns: 1fr; }
  .summary { grid-template-columns: 1fr; }
  .nodes { grid-template-columns: 1fr; }
}
</style>
</head>
<body>
<header>
  <h1>Raft Cluster Dashboard</h1>
  <div class="toolbar">
    <span id="message">Loading...</span>
    <button id="refresh">Refresh</button>
  </div>
</header>
<main>
  <div class="shell">
    <aside class="control-panel">
      <section class="panel">
        <h3>Cluster</h3>
        <div class="forms">
          <input id="peers" type="number" min="1" value="5" aria-label="Peers">
          <button class="primary" id="create">Create Cluster</button>
        </div>
      </section>
      <section class="panel">
        <h3>Write Command</h3>
        <div class="forms">
          <input id="key" placeholder="key" aria-label="Key">
          <input id="value" type="number" placeholder="value" aria-label="Value">
          <button class="primary" id="set">Set</button>
        </div>
      </section>
      <section class="panel">
        <h3>Network Partition</h3>
        <div class="forms">
          <input id="partitionA" placeholder="0,1,2" aria-label="Partition A">
          <input id="partitionB" placeholder="3,4" aria-label="Partition B">
        </div>
        <div class="button-row">
          <button class="primary" id="partition">Apply Partition</button>
          <button id="partitionMajority">Majority Split</button>
          <button id="isolateLeader">Isolate Leader</button>
          <button id="heal">Heal Network</button>
        </div>
        <p>Use a 5-node cluster to see why Raft keeps the majority side available and blocks the minority side from committing writes.</p>
      </section>
    </aside>
    <section class="content">
      <section class="summary">
        <div class="metric good"><span>Quorum Leader</span><strong id="leader">-</strong></div>
        <div class="metric"><span>Term</span><strong id="term">-</strong></div>
        <div class="metric"><span>Nodes</span><strong id="nodeCount">0</strong></div>
        <div class="metric warn"><span>Majority Size</span><strong id="majority">-</strong></div>
      </section>
      <section class="nodes" id="nodes"></section>
    </section>
  </div>
</main>
<script>
const message = document.querySelector("#message");
const nodesEl = document.querySelector("#nodes");
let currentCluster = { initialized: false, nodes: [], leader_id: -1 };

function setMessage(text) {
  message.textContent = text;
}

async function request(path, options = {}) {
  const res = await fetch(path, options);
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

function renderRows(items, emptyText, stateRows = false) {
  if (!items || items.length === 0) return '<div class="empty">' + emptyText + '</div>';
  return items.map(item => {
    if (stateRows) return '<div class="row"><span>' + item[0] + '</span><strong>' + item[1] + '</strong></div>';
    return '<div class="row"><span>#' + item.index + '</span><span>T' + item.term + '</span><strong>' + item.command + '</strong></div>';
  }).join("");
}

function renderNode(node) {
  const state = Object.entries(node.state || {}).sort((a, b) => a[0].localeCompare(b[0]));
  const reachable = (node.reachable || []).join(", ") || "-";
  return '<article class="node">' +
    '<div class="node-head">' +
      '<h2>Node ' + node.id + '</h2>' +
      '<span class="role ' + node.role + '">' + node.role + '</span>' +
    '</div>' +
    '<div class="facts">' +
      '<div>term: <strong>' + node.term + '</strong></div>' +
      '<div>voted for: <strong>' + node.voted_for + '</strong></div>' +
      '<div>commit index: <strong>' + node.commit_index + '</strong></div>' +
      '<div>last applied: <strong>' + node.last_applied + '</strong></div>' +
      '<div>alive: <strong>' + node.alive + '</strong></div>' +
      '<div>connected: <strong>' + node.connected + '</strong></div>' +
      '<div>peers: <strong>' + ((node.peers || []).join(", ") || "-") + '</strong></div>' +
      '<div>can reach: <strong>' + reachable + '</strong></div>' +
      '<div>log length: <strong>' + (node.log || []).length + '</strong></div>' +
    '</div>' +
    '<div class="section-title">Replicated Log</div>' +
    '<div class="log">' + renderRows(node.log, "No log entries yet") + '</div>' +
    '<div class="section-title">State Machine</div>' +
    '<div class="state">' + renderRows(state, "No committed key-values yet", true) + '</div>' +
    '<div class="section-title">Committed Entries</div>' +
    '<div class="log">' + renderRows(node.commits, "No commits applied yet") + '</div>' +
    '<div class="actions">' +
      '<button data-action="disconnect" data-id="' + node.id + '">Disconnect</button>' +
      '<button data-action="reconnect" data-id="' + node.id + '">Reconnect</button>' +
      '<button class="danger" data-action="crash" data-id="' + node.id + '">Crash</button>' +
      '<button data-action="restart" data-id="' + node.id + '">Restart</button>' +
    '</div>' +
  '</article>';
}

async function refresh() {
  try {
    const data = await request("/api/cluster");
    currentCluster = data;
    document.querySelector("#leader").textContent = data.initialized ? data.leader_id : "-";
    document.querySelector("#term").textContent = data.initialized ? data.term : "-";
    document.querySelector("#nodeCount").textContent = data.nodes.length;
    document.querySelector("#majority").textContent = data.nodes.length ? Math.floor(data.nodes.length / 2) + 1 : "-";
    nodesEl.innerHTML = data.nodes.map(renderNode).join("") || '<div class="empty">Create a cluster to begin.</div>';
    setMessage(data.initialized ? "Live snapshot" : "No cluster yet");
  } catch (err) {
    setMessage(err.message.trim());
  }
}

document.querySelector("#create").addEventListener("click", async () => {
  try {
    await request("/api/cluster", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ peers: Number(document.querySelector("#peers").value) })
    });
    await refresh();
  } catch (err) {
    setMessage(err.message.trim());
  }
});

document.querySelector("#set").addEventListener("click", async () => {
  try {
    await request("/set", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        key: document.querySelector("#key").value,
        val: Number(document.querySelector("#value").value)
      })
    });
    setTimeout(refresh, 180);
  } catch (err) {
    setMessage(err.message.trim());
  }
});

function parseGroup(value) {
  return value.split(",")
    .map(part => part.trim())
    .filter(Boolean)
    .map(Number)
    .filter(Number.isInteger);
}

async function applyPartition(groups) {
  await request("/api/network/partition", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ groups })
  });
  setTimeout(refresh, 400);
}

document.querySelector("#partition").addEventListener("click", async () => {
  try {
    await applyPartition([
      parseGroup(document.querySelector("#partitionA").value),
      parseGroup(document.querySelector("#partitionB").value)
    ]);
  } catch (err) {
    setMessage(err.message.trim());
  }
});

document.querySelector("#partitionMajority").addEventListener("click", async () => {
  try {
    const ids = (currentCluster.nodes || []).map(node => node.id);
    const majority = Math.floor(ids.length / 2) + 1;
    await applyPartition([ids.slice(0, majority), ids.slice(majority)]);
  } catch (err) {
    setMessage(err.message.trim());
  }
});

document.querySelector("#isolateLeader").addEventListener("click", async () => {
  try {
    const ids = (currentCluster.nodes || []).map(node => node.id);
    const leader = currentCluster.leader_id;
    if (leader < 0) throw new Error("No quorum leader to isolate");
    await applyPartition([[leader], ids.filter(id => id !== leader)]);
  } catch (err) {
    setMessage(err.message.trim());
  }
});

document.querySelector("#heal").addEventListener("click", async () => {
  try {
    await request("/api/network/heal", { method: "POST" });
    setTimeout(refresh, 400);
  } catch (err) {
    setMessage(err.message.trim());
  }
});

nodesEl.addEventListener("click", async event => {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  try {
    await request("/api/peer/" + button.dataset.id + "/" + button.dataset.action, { method: "POST" });
    setTimeout(refresh, 250);
  } catch (err) {
    setMessage(err.message.trim());
  }
});

document.querySelector("#refresh").addEventListener("click", refresh);
setInterval(refresh, 1000);
refresh();
</script>
</body>
</html>`
