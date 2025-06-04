package main

import (
	"SOTrabalhoFinal/entities"
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

type AgentService struct{}

func (s *AgentService) GetProcesses(_ struct{}, reply *[]entities.ProcessInfo) error {
	processList, err := ps.Processes()
	if err != nil {
		return err
	}
	var resultados []entities.ProcessInfo
	for _, p := range processList {
		name, err := p.Name()
		if err != nil {
			continue
		}
		cpuPercent, _ := p.CPUPercent()
		memPercent, _ := p.MemoryPercent()
		resultados = append(resultados, entities.ProcessInfo{
			PID:    p.Pid,
			Name:   name,
			CPU:    cpuPercent,
			Memory: memPercent,
		})
	}
	*reply = resultados
	return nil
}

func (s *AgentService) KillProcess(args *entities.KillArgs, reply *entities.KillReply) error {
	proc, err := os.FindProcess(int(args.PID))
	if err != nil {
		reply.Success = false
		reply.Message = fmt.Sprintf("PID %d não encontrado: %v", args.PID, err)
		return nil
	}
	err = proc.Kill()
	if err != nil {
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
		log.Fatal("Defina a variável CENTRAL_URL (ex: http://seuVPS:8080/register)")
	}

	// 2. Identificador deste agente (hostname)
	hostname, _ := os.Hostname()
	agentID := hostname

	// 3. Porta em que o agente vai escutar RPC (padrão 9000)
	rpcPort := os.Getenv("AGENT_RPC_PORT")
	if rpcPort == "" {
		rpcPort = "9000"
	}

	// 3.1. Endereço público que será anunciado ao central via AGENT_RPC_ADDR
	publicRPC := os.Getenv("AGENT_RPC_ADDR")
	if publicRPC == "" {
		// Se não definido, cai em "0.0.0.0:9000" (funciona apenas localmente)
		publicRPC = fmt.Sprintf("0.0.0.0:%s", rpcPort)
	}

	// 4. Registrando-se no servidor central (tenta até obter 200 OK)
	go func() {
		for {
			payload := map[string]string{
				"agent_id": agentID,
				"rpc_addr": publicRPC,
			}
			bodyBytes, _ := json.Marshal(payload)
			resp, err := http.Post(centralURL, "application/json", bytes.NewBuffer(bodyBytes))
			if err != nil {
				log.Printf("Falha ao registrar no central: %v. Retentando em 5s...", err)
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

	// 6. Inicia listener TCP para RPC em “0.0.0.0:<porta>”
	listenAddress := fmt.Sprintf("0.0.0.0:%s", rpcPort)
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("Erro ao escutar em %s: %v", listenAddress, err)
	}
	log.Printf("Agente RPC escutando em %s", listenAddress)

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
