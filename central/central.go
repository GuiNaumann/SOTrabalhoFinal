package main

import (
	"SOTrabalhoFinal/entities"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Mapa de agentes (protegido por mutex)
var (
	agentsMu sync.RWMutex
	agents   = map[string]entities.AgentInfo{} // chave: AgentID
)

// --- WebSocket upgrader ---
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Em produção, valide a origem (Origin) conforme necessário.
		return true
	},
}

// --- Handlers HTTP ---

// registerHandler recebe {agent_id, rpc_addr} e armazena no mapa.
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var payload entities.RegisterPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	if payload.AgentID == "" || payload.RPCAddr == "" {
		http.Error(w, "Campos obrigatórios ausentes", http.StatusBadRequest)
		return
	}

	agentsMu.Lock()
	agents[payload.AgentID] = entities.AgentInfo{
		ID:      payload.AgentID,
		RPCAddr: payload.RPCAddr,
	}
	agentsMu.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// listAgentsHandler retorna JSON com todos os agentes registrados.
func listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	agentsMu.RLock()
	defer agentsMu.RUnlock()

	var list []entities.AgentInfo
	for _, a := range agents {
		list = append(list, a)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// getProcessesHandler conecta via RPC ao agente e retorna lista de processos.
func getProcessesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentID := vars["id"]

	agentsMu.RLock()
	info, existe := agents[agentID]
	agentsMu.RUnlock()
	if !existe {
		http.Error(w, "Agente não encontrado", http.StatusNotFound)
		return
	}

	client, err := rpc.Dial("tcp", info.RPCAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao conectar RPC: %v", err), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	var processos []entities.ProcessInfo
	if err = client.Call("AgentService.GetProcesses", struct{}{}, &processos); err != nil {
		http.Error(w, fmt.Sprintf("RPC GetProcesses falhou: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processos)
}

// killProcessHandler faz RPC KillProcess no agente correspondente.
func killProcessHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentID := vars["id"]

	agentsMu.RLock()
	info, existe := agents[agentID]
	agentsMu.RUnlock()
	if !existe {
		http.Error(w, "Agente não encontrado", http.StatusNotFound)
		return
	}

	var args entities.KillArgs
	err := json.NewDecoder(r.Body).Decode(&args)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	client, err := rpc.Dial("tcp", info.RPCAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao conectar RPC: %v", err), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	var reply entities.KillReply
	err = client.Call("AgentService.KillProcess", &args, &reply)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC KillProcess falhou: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// streamProcessesHandler abre um WebSocket e envia, a cada segundo, a lista de processos do agente.
func streamProcessesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentID := vars["id"]

	agentsMu.RLock()
	info, existe := agents[agentID]
	agentsMu.RUnlock()
	if !existe {
		http.Error(w, "Agente não encontrado", http.StatusNotFound)
		return
	}

	// Faz o upgrade da conexão HTTP para WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	// Conecta via RPC ao agente
	client, err := rpc.Dial("tcp", info.RPCAddr)
	if err != nil {
		// Se falhar a conexão RPC, apenas fecha o socket
		return
	}
	defer client.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var processos []entities.ProcessInfo
			err = client.Call("AgentService.GetProcesses", struct{}{}, &processos)
			if err != nil {
				// Se falhar a chamada RPC, encerramos o loop e fechamos o WS
				return
			}
			// Converte para JSON
			data, _ := json.Marshal(processos)
			// Envia pelo WebSocket
			err = wsConn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				// Se erro de escrita (cliente desconectou), encerra loop
				return
			}
		}
	}
}

func main() {
	r := mux.NewRouter()

	// Rota para registro de agentes
	r.HandleFunc("/register", registerHandler).Methods("POST")

	// Rotas da API
	r.HandleFunc("/agents", listAgentsHandler).Methods("GET")
	r.HandleFunc("/agents/{id}/processes", getProcessesHandler).Methods("GET")
	r.HandleFunc("/agents/{id}/kill", killProcessHandler).Methods("POST")

	// Nova rota: WebSocket para streaming de processos
	r.HandleFunc("/agents/{id}/stream", streamProcessesHandler)

	// Servir arquivos estáticos (HTML/JS) em / → ./static/index.html
	fs := http.FileServer(http.Dir("../static"))
	r.PathPrefix("/").Handler(fs)

	addr := ":8080"
	log.Printf("Servidor central ouvindo em %s …", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
