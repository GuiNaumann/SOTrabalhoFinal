package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"time"

	ps "github.com/shirou/gopsutil/v3/process"
)

// --- Estruturas RPC ---

// ProcessInfo representa um processo que será enviado por RPC.
type ProcessInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`    // porcentagem de CPU
	Memory float32 `json:"memory"` // porcentagem de memória
}

// KillArgs recebe o PID a ser finalizado.
type KillArgs struct {
	PID int32 `json:"pid"`
}

// KillReply indica sucesso ou erro ao finalizar.
type KillReply struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// AgentService expõe métodos que podem ser chamados via RPC.
type AgentService struct{}

// GetProcesses retorna a lista de processos atuais (PID, nome, uso de CPU/mem).
func (s *AgentService) GetProcesses(_ struct{}, reply *[]ProcessInfo) error {
	// Primeiro, chama CPUPercent para inicializar as amostras
	processList, err := ps.Processes()
	if err != nil {
		return err
	}

	var resultados []ProcessInfo
	for _, p := range processList {
		name, err := p.Name()
		if err != nil {
			continue
		}
		cpuPercent, _ := p.CPUPercent()
		memPercent, _ := p.MemoryPercent()
		resultados = append(resultados, ProcessInfo{
			PID:    p.Pid,
			Name:   name,
			CPU:    cpuPercent,
			Memory: memPercent,
		})
	}
	*reply = resultados
	return nil
}

// KillProcess tenta finalizar o processo cujo PID foi passado nos args.
func (s *AgentService) KillProcess(args *KillArgs, reply *KillReply) error {
	proc, err := os.FindProcess(int(args.PID))
	if err != nil {
		reply.Success = false
		reply.Message = fmt.Sprintf("PID %d não encontrado: %v", args.PID, err)
		return nil
	}
	if err := proc.Kill(); err != nil {
		reply.Success = false
		reply.Message = fmt.Sprintf("Falha ao finalizar PID %d: %v", args.PID, err)
		return nil
	}
	reply.Success = true
	return nil
}

func main() {
	// 1. Obter URL do servidor central de variável de ambiente
	centralURL := os.Getenv("CENTRAL_URL")
	if centralURL == "" {
		log.Fatal("Defina a variável CENTRAL_URL (ex: http://IP_DO_CENTRAL:8080/register)")
	}

	// 2. Identificador deste agente (hostname)
	hostname, _ := os.Hostname()
	agentID := hostname

	// 3. Porta em que o agente vai escutar RPC
	rpcPort := os.Getenv("AGENT_RPC_PORT")
	if rpcPort == "" {
		rpcPort = "9000"
	}
	rpcAddress := fmt.Sprintf("0.0.0.0:%s", rpcPort)

	// 4. Registrando-se no servidor central (tenta até obter 200 OK)
	go func() {
		for {
			payload := map[string]string{
				"agent_id": agentID,
				"rpc_addr": rpcAddress, // Ex: "192.168.0.10:9000"
			}
			bodyBytes, _ := json.Marshal(payload)
			resp, err := http.Post(centralURL, "application/json", bytes.NewBuffer(bodyBytes))
			if err != nil {
				log.Printf("Falha ao registrar no central: %v. Tentando novamente em 5s...", err)
				time.Sleep(5 * time.Second)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Println("Registrado com sucesso no servidor central.")
				break
			}
			log.Printf("Central retornou status %d. Retentando em 5s...", resp.StatusCode)
			time.Sleep(5 * time.Second)
		}
	}()

	// 5. Registra AgentService como servidor RPC
	agentService := new(AgentService)
	rpc.Register(agentService)

	// 6. Inicia listener TCP para RPC
	listener, err := net.Listen("tcp", rpcAddress)
	if err != nil {
		log.Fatalf("Erro ao escutar em %s: %v", rpcAddress, err)
	}
	log.Printf("Agente RPC escutando em %s", rpcAddress)

	// 7. Aceita conexões RPC indefinidamente
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept erro: %v", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
