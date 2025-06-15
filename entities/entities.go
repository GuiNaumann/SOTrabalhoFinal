package entities

type AgentInfo struct {
	ID      string // ex: hostname
	RPCAddr string // ex: "192.168.0.10:9000"
}

// Payload que o agente envia no POST /register
type RegisterPayload struct {
	AgentID string `json:"agent_id"`
	RPCAddr string `json:"rpc_addr"`
}

// ProcessInfo (igual à do agente, para desserializar resposta RPC)
type ProcessInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`  // só para exibir
	Name2  string  `json:"name2"` // só para exibir
	Path   string  `json:"path"`  // para "start" depois
	CPU    float64 `json:"cpu"`
	Memory float32 `json:"memory"`
}

// Argumentos para KillProcess RPC
type KillArgs struct {
	PID int32 `json:"pid"`
}

// Resposta de KillProcess RPC
type KillReply struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type ServiceReply struct {
	Success bool
	Message string
}
