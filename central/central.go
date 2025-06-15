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

var (
	agentsMu sync.RWMutex
	agents   = map[string]entities.AgentInfo{} // chave: AgentID
)

var killedProcesses = map[string][]entities.ProcessInfo{}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Em produção, valide a origem (Origin) conforme necessário.
		return true
	},
}

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
	err = client.Call("AgentService.GetProcesses", struct{}{}, &processos)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC GetProcesses falhou: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processos)
}

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

	// 1) obter lista atual de processos para capturar o nome
	var procs []entities.ProcessInfo
	err = client.Call("AgentService.GetProcesses", struct{}{}, &procs)
	if err != nil {
		// se falhar, continuamos mesmo assim (vai matar pelo PID)
	}
	procName := ""
	for _, p := range procs {
		if p.PID == args.PID {
			procName = p.Name
			break
		}
	}

	// 2) matar o processo
	var reply entities.KillReply
	err = client.Call("AgentService.KillProcess", &args, &reply)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC KillProcess falhou: %v", err), http.StatusInternalServerError)
		return
	}

	// 3) se matou com sucesso, guarda PID + nome na lista de mortos

	if reply.Success {
		killedProcesses[agentID] = append(killedProcesses[agentID], entities.ProcessInfo{
			PID:  args.PID,
			Name: procName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// startServiceHandler chama RPC StartService no agente
func startServiceHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	agentsMu.RLock()
	info, ok := agents[id]
	agentsMu.RUnlock()
	if !ok {
		http.Error(w, "Agente não encontrado", http.StatusNotFound)
		return
	}

	var payload struct{ Name string }
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	client, err := rpc.Dial("tcp", info.RPCAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	// use o tipo correto aqui:
	var svcReply entities.ServiceReply
	err = client.Call("AgentService.StartService", payload.Name, &svcReply)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": svcReply.Success,
		"message": svcReply.Message,
	})
}

func stopServiceHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	agentsMu.RLock()
	info, ok := agents[id]
	agentsMu.RUnlock()
	if !ok {
		http.Error(w, "Agente não encontrado", http.StatusNotFound)
		return
	}

	var payload struct{ Name string }
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	client, err := rpc.Dial("tcp", info.RPCAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	// use também ServiceReply aqui:
	var svcReply entities.ServiceReply
	err = client.Call("AgentService.StopService", payload.Name, &svcReply)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": svcReply.Success,
		"message": svcReply.Message,
	})
}

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
			data, _ := json.Marshal(processos)
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
	// QUEM CHAMA É O CMD OU POWERHELL NA HORA DE RODAR OS COMANDOS
	r.HandleFunc("/register", registerHandler).Methods("POST")

	// Rotas da API
	// TA TUDO DENTRO DO INDEX.HTML
	r.HandleFunc("/agents", listAgentsHandler).Methods("GET")
	r.HandleFunc("/agents/{id}/processes", getProcessesHandler).Methods("GET")
	r.HandleFunc("/agents/{id}/kill", killProcessHandler).Methods("POST")
	r.HandleFunc("/agents/{id}/start", startServiceHandler).Methods("POST")
	r.HandleFunc("/agents/{id}/stop", stopServiceHandler).Methods("POST")

	// Nova rota: WebSocket para streaming de processos
	// TA TUDO DENTRO DO INDEX.HTML
	r.HandleFunc("/agents/{id}/stream", streamProcessesHandler)

	// Servir arquivos estáticos (HTML/JS) em / → ./static/index.html
	fs := http.FileServer(http.Dir("../static"))
	r.PathPrefix("/").Handler(fs)

	addr := ":8080"
	log.Printf("Servidor central ouvindo em %s …", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
